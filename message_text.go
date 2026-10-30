package main

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
)

const (
	replyItemType    = "http://schema.skype.com/Reply"
	emojiItemType    = "http://schema.skype.com/Emoji"
	imagePlaceholder = "[image]"
)

// blockTags end a run of text: text on either side of them is joined with two
// spaces instead of flowing together.
var blockTags = map[string]struct{}{
	"blockquote": {}, "br": {}, "div": {}, "h1": {}, "h2": {}, "h3": {}, "h4": {}, "h5": {}, "h6": {},
	"li": {}, "ol": {}, "p": {}, "pre": {}, "table": {}, "td": {}, "tr": {}, "ul": {},
}

// messageText flattens a message's HTML body to plain text without resolving
// quoted authors. See renderMessageText.
func messageText(input string) string {
	return renderMessageText(input, nil)
}

// renderMessageText flattens a message's HTML body to plain text. Text runs
// are joined with two spaces. Emoji become their alt text and pictures become
// a placeholder, so a message that is only a picture or an emoji is not blank.
// A quoted reply is rendered on its own line as "> author: text", with the
// author looked up by MRI through nameFor when that is possible, because the
// name Teams embeds in the quote is not always the real one.
func renderMessageText(input string, nameFor func(mri string) string) string {
	var (
		lines []string
		body  textRuns
		quote *replyQuote
	)
	emitBody := func() {
		if text := body.join(); text != "" {
			lines = append(lines, text)
		}
		body = textRuns{}
	}
	activeRuns := func() *textRuns {
		if quote != nil {
			return &quote.preview
		}
		return &body
	}

	z := html.NewTokenizer(bytes.NewBufferString(input))
	for {
		switch z.Next() {
		case html.ErrorToken:
			if quote != nil {
				lines = append(lines, quote.render(nameFor))
			}
			emitBody()
			return strings.Join(lines, "\n")

		case html.StartTagToken, html.SelfClosingTagToken:
			token := z.Token()
			if _, ok := blockTags[token.Data]; ok {
				activeRuns().flush()
			}
			switch token.Data {
			case "blockquote":
				if quote == nil && attrValue(token, "itemtype") == replyItemType {
					emitBody()
					quote = &replyQuote{}
				}
			case "strong":
				if quote != nil && attrValue(token, "itemprop") == "mri" {
					quote.mri = attrValue(token, "itemid")
					quote.inAuthor = true
				}
			case "img":
				if text := imageText(token); text != "" {
					activeRuns().write(text)
				}
			}

		case html.EndTagToken:
			token := z.Token()
			if _, ok := blockTags[token.Data]; ok {
				activeRuns().flush()
			}
			switch token.Data {
			case "strong":
				if quote != nil {
					quote.inAuthor = false
				}
			case "blockquote":
				if quote != nil {
					lines = append(lines, quote.render(nameFor))
					quote = nil
				}
			}

		case html.TextToken:
			text := string(z.Text())
			if quote != nil && quote.inAuthor {
				quote.author += text
				continue
			}
			activeRuns().write(text)
		}
	}
}

// textRuns collects text between block boundaries. Whitespace inside a run is
// collapsed; runs are joined with two spaces.
type textRuns struct {
	current strings.Builder
	parts   []string
}

func (r *textRuns) write(text string) {
	r.current.WriteString(text)
}

func (r *textRuns) flush() {
	if text := collapseSpace(r.current.String()); text != "" {
		r.parts = append(r.parts, text)
	}
	r.current.Reset()
}

func (r *textRuns) join() string {
	r.flush()
	return strings.Join(r.parts, "  ")
}

type replyQuote struct {
	mri      string
	author   string
	inAuthor bool
	preview  textRuns
}

func (q *replyQuote) render(nameFor func(mri string) string) string {
	author := ""
	if nameFor != nil && q.mri != "" {
		author = strings.TrimSpace(nameFor(q.mri))
	}
	if author == "" {
		author = collapseSpace(q.author)
	}
	preview := q.preview.join()

	switch {
	case author == "":
		return "> " + preview
	case preview == "":
		return "> " + author
	default:
		return "> " + author + ": " + preview
	}
}

// imageText returns the text standing in for an <img>: the alt text for an
// emoji, a placeholder for anything else.
func imageText(token html.Token) string {
	if attrValue(token, "itemtype") == emojiItemType {
		if alt := strings.TrimSpace(attrValue(token, "alt")); alt != "" {
			return alt
		}
		if title := strings.TrimSpace(attrValue(token, "title")); title != "" {
			return ":" + title + ":"
		}
	}

	return imagePlaceholder
}

func attrValue(token html.Token, key string) string {
	for _, attr := range token.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func collapseSpace(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
