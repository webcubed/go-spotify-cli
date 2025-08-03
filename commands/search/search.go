package search

import (
	"net/url"

	"github.com/webcubed/go-spotify-cli/commands/cmdTypes"
	"github.com/webcubed/go-spotify-cli/config"

	"github.com/webcubed/go-spotify-cli/commands"
	"github.com/webcubed/go-spotify-cli/commands/player"
	"github.com/webcubed/go-spotify-cli/commands/search/searchPrompt"
	"github.com/webcubed/go-spotify-cli/loader"
	"github.com/webcubed/go-spotify-cli/server"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"strconv"
	"encoding/json"
)

const (
	spotifySearchEndpoint = "https://api.spotify.com/v1/search"
)

func buildSpotifySearchURL(baseEndpoint string, prompt *cmdTypes.SpotifySearchQuery) string {
	values := url.Values{}
	values.Add("q", prompt.Query)
	values.Add("type", prompt.Type)
	values.Add("limit", prompt.Limit)

	fullURL := baseEndpoint + "?" + values.Encode()

	return fullURL
}

func search(cfg *config.Config, accessToken string, query *cmdTypes.SpotifySearchQuery, nextUrl string) {
	loader.Start()
	var endpoint string
	if query != nil {
		endpoint = buildSpotifySearchURL(spotifySearchEndpoint, query)
	} else {
		endpoint = nextUrl
	}

	params := &commands.PlayerParams{
		AccessToken: accessToken,
		Method:      "GET",
		Endpoint:    endpoint,
	}

	body, err := commands.Fetch(params)
	loader.Stop()

	if err != nil {
		switch e := err.(type) {
		case cmdTypes.SpotifyAPIError:
			if e.Detail.Error.Message == "Player command failed: No active device found" {
				player.Device(cfg)
			}
		default:
			logrus.WithError(err).Error("Error searching tracks")
			return
		}

	} else {
		if query.Limit == "1" {
			// If only one result is requested, play it directly
			var result map[string]interface{}
			err = json.Unmarshal(body, &result)
			if err != nil {
				logrus.WithError(err).Error("Error unmarshaling JSON response")
				return
			}
			track := result["tracks"].(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})
			trackUri := track["uri"].(string)
			player.AddToQueue(cfg, accessToken, trackUri)
			player.Next(cfg, accessToken, false)
		} else {
			// Otherwise, prompt the user to select a result
			result := searchPrompt.SpotifySearchResultsPrompt(body)
			if len(result.NextUrl) > 0 {
				search(cfg, accessToken, nil, result.NextUrl)
			}
			if len(result.PlayUrl) > 0 {
				// instead of Calling Play function, we are adding song to the queue and using Next function
				// otherwise song playing further nexts is not possible
				// player.Play(accessToken, playUrl)
				player.AddToQueue(cfg, accessToken, result.PlayUrl)
				player.Next(cfg, accessToken, false)
			}
		}
	}
}

func SendSearchCommand(cfg *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search spotify song",
		Run: func(cmd *cobra.Command, args []string) {
			loader.Stop()
			token := server.ReadUserModifyTokenOrFetchFromServer(cfg)
			var query *cmdTypes.SpotifySearchQuery
			var err error

			// Check if query flag is provided
			queryFlag, err := cmd.Flags().GetString("query")
			if err != nil {
				logrus.WithError(err).Error("Error getting query flag")
				return
			}

			typeFlag, err := cmd.Flags().GetString("type")
			if err != nil {
				logrus.WithError(err).Error("Error getting type flag")
				return
			}

			limitFlag, err := cmd.Flags().GetInt("limit")
			if err != nil {
				logrus.WithError(err).Error("Error getting limit flag")
				return
			}

			if queryFlag != "" {
				// Create a new SpotifySearchQuery with the query flag
				query = &cmdTypes.SpotifySearchQuery{
					Query: queryFlag,
					Type:  typeFlag,
					Limit: strconv.Itoa(limitFlag),
				}
			} else {
				// If query flag is not provided, prompt for search query
				err, query = searchPrompt.SpotifySearchQueryPrompt()
				if err != nil {
					logrus.WithError(err).Error("Error getting Search Query Prompts")
					return
				}
			}

			search(cfg, token, query, "")
		},
	}

	// Add query flag to the command
	cmd.Flags().String("query", "", "Search query")
	cmd.Flags().String("type", "track", "Type of search (e.g. track, episode, artist, album)")
	cmd.Flags().Int("limit", 20, "Number of results to return")

	return cmd
}