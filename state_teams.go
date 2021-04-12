package main

import (
	"fmt"
	teams_api "github.com/fossteams/teams-api"
	"github.com/fossteams/teams-api/pkg/csa"
	"github.com/fossteams/teams-api/pkg/models"
	"sort"
)

type TeamsState struct {
	teamsClient *teams_api.TeamsClient

	conversations  *csa.ConversationResponse
	me             *models.User
	pinnedChannels []csa.ChannelId
	channelById    map[string]Channel
	teamById       map[string]*csa.Team
}

type Channel struct {
	*csa.Channel
	parent *csa.Team
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

	s.pinnedChannels, err = client.GetPinnedChannels()
	if err != nil {
		return fmt.Errorf("unable to get pinned channels: %v", err)
	}

	s.conversations, err = client.GetConversations()
	if err != nil {
		return fmt.Errorf("unable to get conversations: %v", err)
	}

	// Sort Teams by Name
	sort.Sort(csa.TeamsByName(s.conversations.Teams))

	// Create maps
	s.teamById = map[string]*csa.Team{}
	s.channelById = map[string]Channel{}

	for _, t := range s.conversations.Teams {
		s.teamById[t.Id] = &t
		for _, c := range t.Channels {
			s.channelById[c.Id] = Channel{
				Channel: &c,
				parent:  &t,
			}
		}
	}

	return nil
}
