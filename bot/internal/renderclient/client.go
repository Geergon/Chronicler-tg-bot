package renderclient

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"time"
)

type TagType string

const (
	TagOwner  TagType = "owner"
	TagAdmin  TagType = "admin"
	TagMember TagType = "member"
)

type Reply struct {
	AuthorID   int64
	AuthorName string
	Text       string
	Segments   []Segment
}

type Segment struct {
	Text          string
	Bold          bool
	Italic        bool
	Mono          bool
	Underline     bool
	Strikethrough bool
	Spoiler       bool
	Color         color.Color
	CustomEmojiID int64
	EmojiImage    []byte
}

type Message struct {
	AuthorID    int64
	AuthorName  string
	Tag         *Tag
	Avatar      []byte
	Standalone  bool
	Reply       *Reply
	Segments    []Segment
	BubbleColor color.Color
	Media       []MediaItem
}

type MediaItem struct {
	Data    []byte
	Spoiler bool
	Video   bool
	Round   bool
}

type Tag struct {
	TagText string
	TagType TagType
}

// ───────────── JSON-contract with Rust backend ─────────────

type wireReply struct {
	AuthorID   int64         `json:"author_id"`
	AuthorName string        `json:"author_name"`
	Text       string        `json:"text"`
	Segments   []wireSegment `json:"segments,omitempty"`
}

type wireSegment struct {
	Text          string  `json:"text"`
	Bold          bool    `json:"bold,omitempty"`
	Italic        bool    `json:"italic,omitempty"`
	Mono          bool    `json:"mono,omitempty"`
	Underline     bool    `json:"underline,omitempty"`
	Strikethrough bool    `json:"strikethrough,omitempty"`
	Spoiler       bool    `json:"spoiler,omitempty"`
	Color         *string `json:"color,omitempty"`
	EmojiImage    *string `json:"emoji_image,omitempty"`
}

func toWireSegment(store *fileStore, s Segment) wireSegment {
	ws := wireSegment{
		Text: s.Text, Bold: s.Bold, Italic: s.Italic, Mono: s.Mono,
		Underline: s.Underline, Strikethrough: s.Strikethrough, Spoiler: s.Spoiler,
	}
	if s.Color != nil {
		c := hexColor(s.Color)
		ws.Color = &c
	}
	if len(s.EmojiImage) > 0 {
		name := store.add(s.EmojiImage)
		ws.EmojiImage = &name
	}
	return ws
}

func toWireSegments(store *fileStore, segs []Segment) []wireSegment {
	out := make([]wireSegment, 0, len(segs))
	for _, s := range segs {
		out = append(out, toWireSegment(store, s))
	}
	return out
}

type wireMessage struct {
	AuthorID    int64         `json:"author_id"`
	AuthorName  string        `json:"author_name"`
	Tag         *wireTag      `json:"tag,omitempty"`
	Avatar      *string       `json:"avatar,omitempty"`
	Standalone  bool          `json:"standalone"`
	Reply       *wireReply    `json:"reply,omitempty"`
	Segments    []wireSegment `json:"segments"`
	BubbleColor string        `json:"bubble_color"`
	Media       []wireMedia   `json:"media,omitempty"`
}

type wireMedia struct {
	Part    string `json:"part"`
	Spoiler bool   `json:"spoiler,omitempty"`
	Video   bool   `json:"video,omitempty"`
	Round   bool   `json:"round,omitempty"`
}

type wireTag struct {
	Text string `json:"text"`
	Kind string `json:"kind"`
}

type wireRequest struct {
	Messages []wireMessage `json:"messages"`
}

// ───────────── Errors ─────────────

type RenderError struct {
	Status int
	Body   string
}

func (e *RenderError) Error() string {
	return fmt.Sprintf("renderer returned %d: %s", e.Status, e.Body)
}

func IsBadInput(err error) bool {
	var re *RenderError
	return errors.As(err, &re) && re.Status == http.StatusBadRequest
}

// ───────────── Client ─────────────

type Client struct {
	baseURL string
	http    *http.Client
	retries int
}

func New(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 45 * time.Second},
		retries: 2,
	}
}

// Ping check backend /health
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("renderer health: %s", resp.Status)
	}
	return nil
}

// Render returns WebP.
func (c *Client) Render(ctx context.Context, msgs []Message) ([]byte, error) {
	body, contentType, err := buildBody(msgs)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		if attempt > 0 {
			// 503 = render queue full, waiting
			select {
			case <-time.After(time.Duration(attempt) * 300 * time.Millisecond):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		png, err := c.do(ctx, body, contentType)
		if err == nil {
			return png, nil
		}
		lastErr = err

		var re *RenderError
		if !errors.As(err, &re) || re.Status != http.StatusServiceUnavailable {
			return nil, err // repeat only 503
		}
	}
	return nil, lastErr
}

func (c *Client) do(ctx context.Context, body []byte, contentType string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/render", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<10))
		return nil, &RenderError{Status: resp.StatusCode, Body: string(msg)}
	}
	return io.ReadAll(resp.Body)
}

// ───────────── multipart building ─────────────

type fileStore struct {
	byHash map[[32]byte]string
	parts  []filePart
}

type filePart struct {
	name string
	data []byte
}

// add return part name
func (s *fileStore) add(data []byte) string {
	h := sha256.Sum256(data)
	if name, ok := s.byHash[h]; ok {
		return name
	}
	name := fmt.Sprintf("img%d", len(s.parts))
	s.byHash[h] = name
	s.parts = append(s.parts, filePart{name: name, data: data})
	return name
}

func buildBody(msgs []Message) ([]byte, string, error) {
	store := &fileStore{byHash: map[[32]byte]string{}}
	req := wireRequest{Messages: make([]wireMessage, 0, len(msgs))}

	for _, m := range msgs {
		wm := wireMessage{
			AuthorID:    m.AuthorID,
			AuthorName:  m.AuthorName,
			Standalone:  m.Standalone,
			BubbleColor: hexColor(m.BubbleColor),
			Segments:    toWireSegments(store, m.Segments),
		}

		if m.Tag != nil && m.Tag.TagText != "" {
			wm.Tag = &wireTag{Text: m.Tag.TagText, Kind: string(m.Tag.TagType)}
		}
		if len(m.Avatar) > 0 {
			name := store.add(m.Avatar)
			wm.Avatar = &name
		}
		if m.Reply != nil {
			wm.Reply = &wireReply{
				AuthorID:   m.Reply.AuthorID,
				AuthorName: m.Reply.AuthorName,
				Text:       m.Reply.Text,
				Segments:   toWireSegments(store, m.Reply.Segments),
			}
		}
		for _, img := range m.Media {
			if len(img.Data) > 0 {
				wm.Media = append(wm.Media, wireMedia{
					Part:    store.add(img.Data),
					Spoiler: img.Spoiler,
					Video:   img.Video,
					Round:   img.Round,
				})
			}
		}
		req.Messages = append(req.Messages, wm)
	}

	reqJSON, err := json.Marshal(req)
	if err != nil {
		return nil, "", err
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	// request part with explicit Content-Type: application/json
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="request"`)
	h.Set("Content-Type", "application/json")
	pw, err := mw.CreatePart(h)
	if err != nil {
		return nil, "", err
	}
	if _, err := pw.Write(reqJSON); err != nil {
		return nil, "", err
	}

	for _, f := range store.parts {
		fw, err := mw.CreateFormFile(f.name, f.name)
		if err != nil {
			return nil, "", err
		}
		if _, err := fw.Write(f.data); err != nil {
			return nil, "", err
		}
	}

	if err := mw.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), mw.FormDataContentType(), nil
}

func hexColor(c color.Color) string {
	if c == nil {
		return "#000000"
	}
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	if n.A == 255 {
		return fmt.Sprintf("#%02X%02X%02X", n.R, n.G, n.B)
	}
	return fmt.Sprintf("#%02X%02X%02X%02X", n.A, n.R, n.G, n.B)
}
