package main

import (
	"fmt"
	teams_api "github.com/fossteams/teams-api"
	"github.com/fossteams/teams-api/pkg/csa"
	"github.com/fossteams/teams-api/pkg/models"
	"sort"
	"strings"
	"sync"
	"time"
)

type TeamsState struct {
	teamsClient *teams_api.TeamsClient
	clientMu    sync.RWMutex

	conversations      *csa.ConversationResponse
	me                 *models.User
	pinnedChannels     []csa.ChannelId
	pinnedChannelOrder map[string]int
	channelById        map[string]Channel
	teamById           map[string]*csa.Team
	// displayNames caches member MRI -> display name. An empty value records a
	// lookup that returned nothing so it is not retried on every refresh. The
	// map is replaced, never mutated, and namesMu guards the replacement: the
	// background tree refresh reads it while the UI goroutine swaps it.
	displayNames  map[string]string
	namesMu       sync.RWMutex
	nameLookupErr error
	// duplicateChatIDs lists chats the last conversations response repeated.
	duplicateChatIDs []string
}

const profileLookupBatchSize = 100

type Channel struct {
	*csa.Channel
	parent *csa.Team
}

type ConversationTarget struct {
	ID    string
	Title string
	Kind  string
}

const (
	targetKindChannel = "channel"
	targetKindChat    = "chat"
	targetKindMeeting = "meeting"
)

func isMeetingChat(chat csa.Chat) bool {
	return strings.EqualFold(strings.TrimSpace(chat.ChatType), "meeting")
}

type conversationData struct {
	conversations      *csa.ConversationResponse
	pinnedChannels     []csa.ChannelId
	pinnedChannelOrder map[string]int
	channelById        map[string]Channel
	teamById           map[string]*csa.Team
	displayNames       map[string]string
	nameLookupErr      error
	duplicateChatIDs   []string
}

func (s *TeamsState) client() *teams_api.TeamsClient {
	s.clientMu.RLock()
	defer s.clientMu.RUnlock()
	return s.teamsClient
}

func (s *TeamsState) setClient(client *teams_api.TeamsClient) {
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	s.teamsClient = client
}

func (s *TeamsState) init(client *teams_api.TeamsClient) error {
	if client == nil {
		return fmt.Errorf("client is nil")
	}
	s.setClient(client)

	var err error
	s.me, err = client.GetMe()
	if err != nil {
		return fmt.Errorf("unable to get your profile: %v", err)
	}

	data, err := s.fetchConversationData()
	if err != nil {
		return err
	}
	s.applyConversationData(data)

	return nil
}

func (s *TeamsState) fetchConversationData() (*conversationData, error) {
	client := s.client()
	if client == nil {
		return nil, fmt.Errorf("teams client is nil")
	}

	pinnedChannels, err := client.GetPinnedChannels()
	if err != nil {
		return nil, fmt.Errorf("unable to get pinned channels: %v", err)
	}

	conversations, err := client.GetConversations()
	if err != nil {
		return nil, fmt.Errorf("unable to get conversations: %v", err)
	}

	data := &conversationData{
		conversations:      conversations,
		pinnedChannels:     pinnedChannels,
		pinnedChannelOrder: map[string]int{},
		channelById:        map[string]Channel{},
		teamById:           map[string]*csa.Team{},
	}

	for idx, channelID := range pinnedChannels {
		data.pinnedChannelOrder[string(channelID)] = idx
	}

	conversations.Chats, data.duplicateChatIDs = dedupeChats(conversations.Chats)

	data.displayNames, data.nameLookupErr = s.resolveDisplayNames(conversations.Chats)

	sortTeams(data.conversations.Teams)
	sortChats(data.conversations.Chats, s.selfMri(), s.selfDisplayName(), data.displayNames)

	for teamIdx := range data.conversations.Teams {
		team := &data.conversations.Teams[teamIdx]
		sortChannels(team.Channels, data.pinnedChannelOrder)
		data.teamById[team.Id] = team
		for channelIdx := range team.Channels {
			channel := &team.Channels[channelIdx]
			data.channelById[channel.Id] = Channel{
				Channel: channel,
				parent:  team,
			}
		}
	}

	return data, nil
}

func (s *TeamsState) applyConversationData(data *conversationData) {
	if data == nil {
		return
	}

	s.conversations = data.conversations
	s.pinnedChannels = data.pinnedChannels
	s.pinnedChannelOrder = data.pinnedChannelOrder
	s.channelById = data.channelById
	s.teamById = data.teamById
	s.setDisplayNames(data.displayNames)
	s.nameLookupErr = data.nameLookupErr
	s.duplicateChatIDs = data.duplicateChatIDs
}

// dedupeChats drops repeated chat entries. The aggregator occasionally lists
// one conversation twice, which showed up as the same chat twice in the tree.
// The entry with the newest activity wins so the latest message is kept.
// Returns the surviving chats in their original order plus the repeated IDs.
func dedupeChats(chats []csa.Chat) ([]csa.Chat, []string) {
	indexByID := map[string]int{}
	result := make([]csa.Chat, 0, len(chats))
	var duplicates []string
	for _, chat := range chats {
		if chat.Id == "" {
			result = append(result, chat)
			continue
		}
		if idx, seen := indexByID[chat.Id]; seen {
			if len(duplicates) == 0 || duplicates[len(duplicates)-1] != chat.Id {
				duplicates = append(duplicates, chat.Id)
			}
			if chatActivityTime(chat).After(chatActivityTime(result[idx])) {
				result[idx] = chat
			}
			continue
		}
		indexByID[chat.Id] = len(result)
		result = append(result, chat)
	}

	return result, duplicates
}

// resolveDisplayNames returns the cached name map extended with lookups for any
// chat members the conversations response left unnamed. Lookup failures are
// returned alongside the partial map rather than aborting the refresh.
func (s *TeamsState) resolveDisplayNames(chats []csa.Chat) (map[string]string, error) {
	known := s.displayNameMap()
	names := make(map[string]string, len(known))
	for mri, name := range known {
		names[mri] = name
	}

	missing := unresolvedMemberMris(chats, s.selfMri(), names)
	client := s.client()
	if len(missing) == 0 || client == nil {
		return names, nil
	}

	var firstErr error
	for _, batch := range chunkStrings(missing, profileLookupBatchSize) {
		users, err := client.FetchShortProfile(batch)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("profile lookup failed for %d members: %v", len(batch), err)
			}
			continue
		}
		for _, mri := range batch {
			names[mri] = ""
		}
		for _, user := range users {
			if user.Mri != "" && strings.TrimSpace(user.DisplayName) != "" {
				names[user.Mri] = strings.TrimSpace(user.DisplayName)
			}
		}
	}

	return names, firstErr
}

// unresolvedMemberMris lists, sorted and de-duplicated, the members of
// non-meeting chats that have no friendly name and are not already in known.
func unresolvedMemberMris(chats []csa.Chat, selfMri string, known map[string]string) []string {
	seen := map[string]struct{}{}
	mris := []string{}
	for _, chat := range chats {
		if isMeetingChat(chat) {
			continue
		}
		for _, member := range chat.Members {
			if member.Mri == "" || member.Mri == selfMri || strings.TrimSpace(member.FriendlyName) != "" {
				continue
			}
			if _, ok := known[member.Mri]; ok {
				continue
			}
			if _, ok := seen[member.Mri]; ok {
				continue
			}
			seen[member.Mri] = struct{}{}
			mris = append(mris, member.Mri)
		}
	}
	sort.Strings(mris)
	return mris
}

func chunkStrings(items []string, size int) [][]string {
	if size <= 0 {
		size = len(items)
	}
	chunks := [][]string{}
	for start := 0; start < len(items); start += size {
		end := start + size
		if end > len(items) {
			end = len(items)
		}
		chunks = append(chunks, items[start:end])
	}
	return chunks
}

// displayNameMap returns the current name cache. Callers only read it; a
// refresh installs a new map through setDisplayNames instead of editing it.
func (s *TeamsState) displayNameMap() map[string]string {
	s.namesMu.RLock()
	defer s.namesMu.RUnlock()
	return s.displayNames
}

func (s *TeamsState) setDisplayNames(names map[string]string) {
	s.namesMu.Lock()
	defer s.namesMu.Unlock()
	s.displayNames = names
}

func (s *TeamsState) selfMri() string {
	if s.me == nil {
		return ""
	}

	return s.me.Mri
}

func (s *TeamsState) selfDisplayName() string {
	if s.me == nil {
		return ""
	}

	return s.me.DisplayName
}

// displayNameFor resolves a member MRI to a display name from the profile
// cache, or to your own name for your own MRI. Unknown MRIs yield "".
func (s *TeamsState) displayNameFor(mri string) string {
	if mri == "" {
		return ""
	}
	if mri == s.selfMri() {
		return s.selfDisplayName()
	}

	return strings.TrimSpace(s.displayNameMap()[mri])
}

func sortTeams(teams []csa.Team) {
	sort.SliceStable(teams, func(i, j int) bool {
		return lessTeam(teams[i], teams[j])
	})
}

func lessTeam(left, right csa.Team) bool {
	if left.IsDeleted != right.IsDeleted {
		return !left.IsDeleted && right.IsDeleted
	}
	if left.IsFavorite != right.IsFavorite {
		return left.IsFavorite && !right.IsFavorite
	}
	if left.IsFollowed != right.IsFollowed {
		return left.IsFollowed && !right.IsFollowed
	}
	if left.IsArchived != right.IsArchived {
		return !left.IsArchived && right.IsArchived
	}

	return normalizedSortText(left.DisplayName) < normalizedSortText(right.DisplayName)
}

func sortChannels(channels []csa.Channel, pinnedChannelOrder map[string]int) {
	sort.SliceStable(channels, func(i, j int) bool {
		return lessChannel(channels[i], channels[j], pinnedChannelOrder)
	})
}

func lessChannel(left, right csa.Channel, pinnedChannelOrder map[string]int) bool {
	leftPinnedIndex, leftPinned := pinnedChannelOrder[left.Id]
	rightPinnedIndex, rightPinned := pinnedChannelOrder[right.Id]
	if leftPinned != rightPinned {
		return leftPinned && !rightPinned
	}
	if leftPinned && rightPinned && leftPinnedIndex != rightPinnedIndex {
		return leftPinnedIndex < rightPinnedIndex
	}
	if left.IsGeneral != right.IsGeneral {
		return left.IsGeneral && !right.IsGeneral
	}
	if left.IsFavorite != right.IsFavorite {
		return left.IsFavorite && !right.IsFavorite
	}
	if left.IsPinned != right.IsPinned {
		return left.IsPinned && !right.IsPinned
	}
	if left.IsArchived != right.IsArchived {
		return !left.IsArchived && right.IsArchived
	}

	return normalizedSortText(left.DisplayName) < normalizedSortText(right.DisplayName)
}

func sortChats(chats []csa.Chat, selfMri, selfDisplayName string, displayNames map[string]string) {
	sort.SliceStable(chats, func(i, j int) bool {
		return lessChat(chats[i], chats[j], selfMri, selfDisplayName, displayNames)
	})
}

// lessChat orders by recency. The API's "hidden" flag is deliberately ignored:
// current Teams sets it on effectively every direct and group chat, so using it
// buries the conversations people actually want behind hundreds of meeting chats.
func lessChat(left, right csa.Chat, selfMri, selfDisplayName string, displayNames map[string]string) bool {
	if left.IsSticky != right.IsSticky {
		return left.IsSticky && !right.IsSticky
	}

	leftTime := chatActivityTime(left)
	rightTime := chatActivityTime(right)
	if !leftTime.Equal(rightTime) {
		return leftTime.After(rightTime)
	}

	leftTitle := normalizedSortText(resolveChatTitleWith(left, selfMri, selfDisplayName, displayNames))
	rightTitle := normalizedSortText(resolveChatTitleWith(right, selfMri, selfDisplayName, displayNames))
	if leftTitle != rightTitle {
		return leftTitle < rightTitle
	}

	return left.Id < right.Id
}

func chatActivityTime(chat csa.Chat) time.Time {
	if ts := time.Time(chat.LastMessage.ComposeTime); !ts.IsZero() {
		return ts
	}
	if ts := time.Time(chat.LastMessage.OriginalArrivalTime); !ts.IsZero() {
		return ts
	}
	if !chat.LastJoinAt.IsZero() {
		return chat.LastJoinAt
	}
	if !chat.LastLeaveAt.IsZero() {
		return chat.LastLeaveAt
	}
	if ts := parseConversationTime(chat.CreatedAt); !ts.IsZero() {
		return ts
	}

	return time.Time{}
}

func parseConversationTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}

	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.9999999Z07:00",
	}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, raw)
		if err == nil {
			return parsed
		}
	}

	return time.Time{}
}

func normalizedSortText(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}
