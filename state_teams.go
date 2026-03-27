package main

import (
	"fmt"
	teams_api "github.com/fossteams/teams-api"
	"github.com/fossteams/teams-api/pkg/csa"
	"github.com/fossteams/teams-api/pkg/models"
	"sort"
	"strings"
	"time"
)

type TeamsState struct {
	teamsClient *teams_api.TeamsClient

	conversations      *csa.ConversationResponse
	me                 *models.User
	pinnedChannels     []csa.ChannelId
	pinnedChannelOrder map[string]int
	channelById        map[string]Channel
	teamById           map[string]*csa.Team
}

type Channel struct {
	*csa.Channel
	parent *csa.Team
}

type ConversationTarget struct {
	ID    string
	Title string
}

type conversationData struct {
	conversations      *csa.ConversationResponse
	pinnedChannels     []csa.ChannelId
	pinnedChannelOrder map[string]int
	channelById        map[string]Channel
	teamById           map[string]*csa.Team
}

func (s *TeamsState) init(client *teams_api.TeamsClient) error {
	if client == nil {
		return fmt.Errorf("client is nil")
	}

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
	if s.teamsClient == nil {
		return nil, fmt.Errorf("teams client is nil")
	}

	pinnedChannels, err := s.teamsClient.GetPinnedChannels()
	if err != nil {
		return nil, fmt.Errorf("unable to get pinned channels: %v", err)
	}

	conversations, err := s.teamsClient.GetConversations()
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

	sortTeams(data.conversations.Teams)
	sortChats(data.conversations.Chats, s.selfMri(), s.selfDisplayName())

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

func sortChats(chats []csa.Chat, selfMri, selfDisplayName string) {
	sort.SliceStable(chats, func(i, j int) bool {
		return lessChat(chats[i], chats[j], selfMri, selfDisplayName)
	})
}

func lessChat(left, right csa.Chat, selfMri, selfDisplayName string) bool {
	if left.Hidden != right.Hidden {
		return !left.Hidden && right.Hidden
	}
	if left.IsSticky != right.IsSticky {
		return left.IsSticky && !right.IsSticky
	}

	leftTime := chatActivityTime(left)
	rightTime := chatActivityTime(right)
	if !leftTime.Equal(rightTime) {
		return leftTime.After(rightTime)
	}

	leftTitle := normalizedSortText(resolveChatTitle(left, selfMri, selfDisplayName))
	rightTitle := normalizedSortText(resolveChatTitle(right, selfMri, selfDisplayName))
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
