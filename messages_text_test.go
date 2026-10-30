package main

import (
	"testing"
)

func TestMessageTextShowsImagePlaceholderForPictures(t *testing.T) {
	content := `<p><img src="https://us-api.asm.skype.com/v1/objects/0-eus-d19-abc/views/imgo" itemtype="http://schema.skype.com/AMSImage" itemscope="png" width="216" height="250" alt="image" id="x_0-eus-d19-abc" itemid="0-eus-d19-abc"></p>`

	if got := messageText(content); got != "[image]" {
		t.Fatalf("expected image placeholder, got %q", got)
	}
}

func TestMessageTextShowsEmojiAltText(t *testing.T) {
	content := `<p><span title="Smile" type="(smile)" class="animated-emoticon-20-smile" itemscope=""><img itemscope="" itemtype="http://schema.skype.com/Emoji" itemid="smile" src="https://statics.teams.cdn.office.net/evergreen-assets/personal-expressions/v2/assets/emoticons/smile/default/20_f.png" title="Smile" alt="🙂" style="width:20px; height:20px"></span></p>`

	if got := messageText(content); got != "🙂" {
		t.Fatalf("expected emoji alt text, got %q", got)
	}
}

func TestMessageTextKeepsEmojiInlineWithText(t *testing.T) {
	content := `<p>nice <img itemtype="http://schema.skype.com/Emoji" itemid="smile" alt="🙂"> thanks</p>`

	if got := messageText(content); got != "nice 🙂 thanks" {
		t.Fatalf("expected emoji inline with surrounding text, got %q", got)
	}
}

const replyQuoteContent = `<blockquote itemscope itemtype="http://schema.skype.com/Reply" itemid="1793112905718"><strong itemprop="mri" itemid="8:orgid:self">Display Name</strong><span itemprop="time" itemid="1793112905718"></span><p itemprop="preview">testing a tool, are you seeing this?</p></blockquote>
<p>?</p>`

func TestMessageTextRendersReplyQuoteWithResolvedAuthor(t *testing.T) {
	nameFor := func(mri string) string {
		if mri == "8:orgid:self" {
			return "Tucker, Michael (Services)"
		}
		return ""
	}

	got := renderMessageText(replyQuoteContent, nameFor)

	want := "> Tucker, Michael (Services): testing a tool, are you seeing this?\n?"
	if got != want {
		t.Fatalf("expected quoted reply on its own line with resolved author, got %q", got)
	}
}

func TestMessageTextRendersReplyQuoteWithFallbackAuthor(t *testing.T) {
	got := messageText(replyQuoteContent)

	want := "> Display Name: testing a tool, are you seeing this?\n?"
	if got != want {
		t.Fatalf("expected quoted reply with the author text from the quote, got %q", got)
	}
}

func TestMessageTextStillJoinsParagraphs(t *testing.T) {
	if got := messageText("<p>one</p><p>two</p>"); got != "one  two" {
		t.Fatalf("expected paragraphs joined with two spaces, got %q", got)
	}
}
