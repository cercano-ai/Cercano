//go:build !realcloudacceptance

package responses

import "net/http"

func acceptanceHTTPClient(_ Config, client *http.Client) *http.Client { return client }
