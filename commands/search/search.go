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
	"fmt"
	"strings"
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
		logrus.WithError(err).Error("Error searching tracks")
		return
	}

	if query.Limit == "1" {
		var result map[string]interface{}
		err = json.Unmarshal(body, &result)
		if err != nil {
			logrus.WithError(err).Error("Error unmarshaling JSON response")
			return
		}

		switch query.Type {
		case "track":
			tracks := result["tracks"].(map[string]interface{})
			if tracks != nil {
				items := tracks["items"].([]interface{})
				if len(items) > 0 {
					track := items[0]
					if track != nil {
						trackMap, ok := track.(map[string]interface{})
						if ok {
							trackUri, ok := trackMap["uri"].(string)
							if ok {
								player.AddToQueue(cfg, accessToken, trackUri)
								player.Next(cfg, accessToken, false)
							}
						}
					}
				}
			}
		case "album":
			albums := result["albums"].(map[string]interface{})
			if albums != nil {
				items := albums["items"].([]interface{})
				if len(items) > 0 {
					album := items[0]
					if album != nil {
						albumMap, ok := album.(map[string]interface{})
						if ok {
							albumUri, ok := albumMap["uri"].(string)
							if ok {
								tracks, err := getAlbumTracks(cfg, accessToken, albumUri)
								if err != nil {
									logrus.WithError(err).Error("Error getting album tracks")
									return
								}
								if len(tracks) > 0 {
									player.AddToQueue(cfg, accessToken, tracks[0])
									player.Next(cfg, accessToken, false)
								}
							}
						}
					}
				}
			}
		default:
			logrus.Errorf("Unsupported type: %s", query.Type)
		}
	} else {
		result := searchPrompt.SpotifySearchResultsPrompt(body)
		if len(result.NextUrl) > 0 {
			search(cfg, accessToken, nil, result.NextUrl)
		}
		if len(result.PlayUrl) > 0 {
			player.AddToQueue(cfg, accessToken, result.PlayUrl)
			player.Next(cfg, accessToken, false)
		}
	}
}

func getAlbumTracks(cfg *config.Config, accessToken string, albumUri string) ([]string, error) {
	params := &commands.PlayerParams{
		AccessToken: accessToken,
		Method:      "GET",
		Endpoint:    fmt.Sprintf("/v1/albums/%s/tracks", strings.Split(albumUri, ":")[2]),
	}

	body, err := commands.Fetch(params)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, err
	}

	tracks := result["items"].([]interface{})
	trackUris := make([]string, len(tracks))
	for i, track := range tracks {
		trackMap, ok := track.(map[string]interface{})
		if ok {
			trackUri, ok := trackMap["uri"].(string)
			if ok {
				trackUris[i] = trackUri
			}
		}
	}
	return trackUris, nil
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