package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// TelegramBot says who a token belongs to.
type TelegramBot struct {
	Username string `json:"username"`
	Name     string `json:"name"`
}

// TelegramChat is a chat a bot has heard from.
type TelegramChat struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"` // private, group, supergroup, channel
}

// DiscoverTelegram checks a bot token, and lists the chats that have written to
// the bot lately, latest first, so nobody has to look up a chat id. Telegram
// keeps messages for a day, and reading them without an offset leaves them there.
func DiscoverTelegram(ctx context.Context, token string, opts Options) (TelegramBot, []TelegramChat, error) {
	s, err := New(TypeTelegram, Config{Token: token, ChatID: "-"}, opts)
	if err != nil {
		return TelegramBot{}, nil, err
	}
	var me struct {
		Username  string `json:"username"`
		FirstName string `json:"first_name"`
	}
	if err := s.telegramGet(ctx, "getMe", &me); err != nil {
		return TelegramBot{}, nil, err
	}
	bot := TelegramBot{Username: me.Username, Name: me.FirstName}
	var updates []map[string]json.RawMessage
	if err := s.telegramGet(ctx, "getUpdates?limit=100&timeout=0", &updates); err != nil {
		return bot, nil, err
	}
	chats := []TelegramChat{}
	seen := map[string]bool{}
	for i := len(updates) - 1; i >= 0; i-- {
		for _, key := range []string{"message", "edited_message", "channel_post", "edited_channel_post", "my_chat_member"} {
			chat, ok := chatOf(updates[i][key])
			if ok && !seen[chat.ID] {
				seen[chat.ID] = true
				chats = append(chats, chat)
			}
		}
	}
	return bot, chats, nil
}

func chatOf(raw json.RawMessage) (TelegramChat, bool) {
	if raw == nil {
		return TelegramChat{}, false
	}
	var holder struct {
		Chat struct {
			ID        json.Number `json:"id"`
			Type      string      `json:"type"`
			Title     string      `json:"title"`
			Username  string      `json:"username"`
			FirstName string      `json:"first_name"`
			LastName  string      `json:"last_name"`
		} `json:"chat"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber() // group ids do not survive a float
	if dec.Decode(&holder) != nil || holder.Chat.ID == "" {
		return TelegramChat{}, false
	}
	c := holder.Chat
	title := c.Title
	if title == "" {
		title = strings.TrimSpace(c.FirstName + " " + c.LastName)
	}
	if title == "" {
		title = c.Username
	}
	if title == "" {
		title = c.ID.String()
	}
	return TelegramChat{ID: c.ID.String(), Title: title, Type: c.Type}, true
}

func (s *Sender) telegramGet(ctx context.Context, method string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.tgAPI+"/bot"+s.config.Token+"/"+method, nil)
	if err != nil {
		return errors.New("invalid token")
	}
	res, err := s.client.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return fmt.Errorf("cannot reach Telegram: %s", s.scrub(err.Error()))
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	var answer struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		Description string          `json:"description"`
	}
	json.Unmarshal(data, &answer)
	switch {
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusNotFound:
		return errors.New("Telegram does not know this token. Copy it again from @BotFather")
	case res.StatusCode == http.StatusConflict:
		return errors.New("something else is reading this bot's messages (another program, or a web hook), so chats cannot be listed here. Enter the chat ID by hand, or use a bot of its own for notifications")
	case !answer.OK:
		return errors.New("Telegram answered " + strconv.Itoa(res.StatusCode) + ": " + s.scrub(answer.Description))
	}
	return json.Unmarshal(answer.Result, out)
}
