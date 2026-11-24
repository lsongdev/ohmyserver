package subscription

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

func Fetch(ctx context.Context, address string) ([]Node, []string, error) {
	if err := ValidateURL(address); err != nil {
		return nil, nil, err
	}
	client := &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 5 || (r.URL.Scheme != "http" && r.URL.Scheme != "https") {
			return fmt.Errorf("invalid redirect")
		}
		return nil
	}}
	req, err := http.NewRequestWithContext(ctx, "GET", address, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid subscription URL")
	}
	req.Header.Set("User-Agent", "Shadowrocket")
	res, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("could not fetch subscription")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("subscription server returned HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 4<<20+1))
	if err != nil {
		return nil, nil, fmt.Errorf("could not read subscription")
	}
	if len(b) > 4<<20 {
		return nil, nil, fmt.Errorf("subscription exceeds 4 MiB")
	}
	return Parse(b)
}

func ValidateURL(address string) error {
	u, err := url.Parse(address)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || len(address) > 8192 {
		return fmt.Errorf("enter an HTTP or HTTPS subscription URL")
	}
	return nil
}
