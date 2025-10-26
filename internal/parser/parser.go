package parser

import (
	"bytes"
	"net/url"
	"regexp"
	"strings"
	"wb_lvl2_wget/internal/urlutil"

	"golang.org/x/net/html"
)

func ExtractLinksAndResources(base *url.URL, body []byte) (links []string, resources []string, modified []byte, err error) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, nil, nil, err
	}

	addURL := func(slice *[]string, raw string) {
		if raw == "" {
			return
		}
		if strings.HasPrefix(raw, "data:") || strings.HasPrefix(raw, "javascript:") {
			return
		}
		if u, err := urlutil.ResolveURL(base, raw); err == nil {
			*slice = append(*slice, u.String())
		}
	}

	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "a":
				for _, a := range n.Attr {
					if a.Key == "href" {
						addURL(&links, a.Val)
					}
				}
			case "img", "script", "iframe", "source", "audio", "video":
				for _, a := range n.Attr {
					if a.Key == "src" {
						addURL(&resources, a.Val)
					}
				}
			case "link":
				var href, rel string
				for _, a := range n.Attr {
					switch a.Key {
					case "rel":
						rel = a.Val
					case "href":
						href = a.Val
					}
				}
				if href != "" && (strings.Contains(rel, "icon") ||
					rel == "stylesheet" ||
					rel == "manifest" ||
					rel == "mask-icon" ||
					strings.Contains(href, ".svg")) {
					addURL(&resources, href)
				}
			case "meta":
				for _, a := range n.Attr {
					if a.Key == "content" && (strings.Contains(a.Val, ".png") ||
						strings.Contains(a.Val, ".json") ||
						strings.Contains(a.Val, ".svg")) {
						addURL(&resources, a.Val)
					}
				}
			case "object", "embed":
				for _, a := range n.Attr {
					if a.Key == "data" {
						addURL(&resources, a.Val)
					}
				}
			}
			for _, a := range n.Attr {
				if strings.HasPrefix(a.Key, "data-") && (strings.Contains(a.Val, "/") || strings.Contains(a.Val, ".")) {
					addURL(&resources, a.Val)
				}
				if a.Key == "poster" {
					addURL(&resources, a.Val)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}

	visit(doc)

	svgRe := regexp.MustCompile(`url\(["']?([^"')]+\.svg)["']?\)`)
	matches := svgRe.FindAllSubmatch(body, -1)
	for _, m := range matches {
		if len(m) > 1 {
			addURL(&resources, string(m[1]))
		}
	}

	return links, resources, body, nil
}
