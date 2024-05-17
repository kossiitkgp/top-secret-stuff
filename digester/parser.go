package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

type Block struct {
	Type     string    `json:"type"`
	Elements []Element `json:"elements"`
}

type Element struct {
	Type      string    `json:"type"`
	Elements  []Element `json:"elements"`
	Text      string    `json:"text"`
	Style     Style     `json:"style"`
	Indent    int       `json:"indent"`
	Border    int       `json:"border"`
	EmojiName string    `json:"name"`
	URL       string    `json:"url"`
	UserID    string    `json:"user_id"`
	ChannelID string    `json:"channel_id"`
}

type Style struct {
	IsList    bool
	ListStyle string
	TextStyle TextStyle
}

type TextStyle struct {
	Bold   bool `json:"bold"`
	Italic bool `json:"italic"`
	Strike bool `json:"strike"`
	Code   bool `json:"code"`
}

func (s *Style) UnmarshalJSON(data []byte) error {
	var listStyle string
	if err := json.Unmarshal(data, &listStyle); err == nil {
		s.IsList = true
		s.ListStyle = listStyle
		return nil
	}

	var textStyle TextStyle
	if err := json.Unmarshal(data, &textStyle); err == nil {
		s.IsList = false
		s.TextStyle = textStyle
		return nil
	}

	return fmt.Errorf("unknown style type")
}

func parseText(element Element) string {
	result := ""

	if element.Type != "text" {
		fmt.Println("[WARNING] Element is not text")
		return result
	}

	if element.Style.IsList {
		fmt.Println("[WARNING] List element tried to be parsed as text")
		return result
	}

	leadingSpacesCount := 0
	for _, char := range element.Text {
		if char == ' ' {
			leadingSpacesCount++
		} else {
			break
		}
	}

	if leadingSpacesCount == len(element.Text) {
		return element.Text
	}

	trailingSpacesCount := 0
	for i := len(element.Text) - 1; i >= 0; i-- {
		if element.Text[i] == ' ' {
			trailingSpacesCount++
		} else {
			break
		}
	}

	result = element.Text[leadingSpacesCount : len(element.Text)-trailingSpacesCount]

	if element.Style.TextStyle.Bold {
		result = "**" + result + "**"
	}
	if element.Style.TextStyle.Italic {
		result = "*" + result + "*"
	}
	if element.Style.TextStyle.Strike {
		result = "~~" + result + "~~"
	}
	if element.Style.TextStyle.Code {
		result = "`" + result + "`"
	}

	for i := 0; i < leadingSpacesCount; i++ {
		result = " " + result
	}

	for i := 0; i < trailingSpacesCount; i++ {
		result = result + " "
	}

	return result
}

func addBorder(text string, border int) string {
	result := ""

	for i := 0; i < border; i++ {
		result += ">"
	}

	if border != 0 {
		result += " "
	}

	return result + text
}

func addIndent(text string, indent int) string {
	result := ""

	for i := 0; i < indent; i++ {
		result += "   "
	}

	return result + text
}

func parseList(element Element) string {
	result := "\n"

	if !element.Style.IsList {
		fmt.Println("[WARNING] Element is not list")
		return result
	}

	if element.Style.ListStyle == "ordered" {
		for index, subElement := range element.Elements {
			result += addBorder(addIndent(fmt.Sprint((index+1))+". "+parseElement(subElement)+"\n", element.Indent), element.Border)
			if element.Border != 0 {
				result += "\n"
			}
		}
	} else {
		for _, subElement := range element.Elements {
			result += addBorder(addIndent("- "+parseElement(subElement)+"\n", element.Indent), element.Border)
			if element.Border != 0 {
				result += "\n"
			}
		}
	}

	return result
}

func parseQuote(element Element) string {
	result := ""

	for _, subElement := range element.Elements {
		result += parseElement(subElement)
	}
	result = strings.ReplaceAll(result, "\n", "\n> ")

	return addBorder(result, 1) + "\n\n"
}

func parsePreformatted(element Element) string {
	result := "```\n"
	for _, subElement := range element.Elements {
		result += parseElement(subElement)
	}
	result += "\n```"

	return result
}

func parseUser(element Element) string {
	result := "@"
	for _, user := range users {
		if user.ID == element.UserID {
			result += user.Name
		}
	}
	if result == "@" {
		result += "unknown-user"
	}
	return "<div class=\"user-mention\">" + result + "</div>"
}

func parseChannel(element Element) string {
	result := "#" + element.ChannelID
	if result == "#" {
		result += "unknown-channel"
	}
	return "<div class=\"channel-mention\">" + result + "</div>"
}

func parseElement(element Element) string {
	result := ""

	switch element.Type {
	case "text":
		result = parseText(element)
	case "emoji":
		result = ":" + element.EmojiName + ":"
	case "user":
		result = parseUser(element)
	case "channel":
		result = parseChannel(element)
	case "link":
		result = "[" + element.Text + "](" + element.URL + ")"
	case "rich_text_section":
		for _, subElement := range element.Elements {
			result += parseElement(subElement)
		}
	case "rich_text_list":
		result += parseList(element)
	case "rich_text_quote":
		result = parseQuote(element)
	case "rich_text_preformatted":
		result = parsePreformatted(element)
	default:
		fmt.Println("Unknown element type: " + element.Type)
	}

	return result
}

func parseBlock(block Block) string {
	result := ""

	for _, element := range block.Elements {
		result += parseElement(element)
	}

	return result
}

func parse(blocks []Block) string {
	result := ""

	for _, block := range blocks {
		result += parseBlock(block) + "\n\n"
	}

	extensions := parser.CommonExtensions
	parser := parser.NewWithExtensions(extensions)
	doc := parser.Parse([]byte(strings.TrimSpace(result)))

	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)

	return string(markdown.Render(doc, renderer))
}
