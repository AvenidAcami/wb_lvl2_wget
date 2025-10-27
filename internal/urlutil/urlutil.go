package urlutil

import (
	"net/url"
	"path"
	"strings"
)

func SameDomain(a, b *url.URL) bool {
	return a != nil && b != nil && a.Hostname() == b.Hostname()
}

func ResolveURL(base *url.URL, href string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return nil, err
	}
	return base.ResolveReference(u), nil
}

func CleanPathForFile(u *url.URL, isHTML bool) string {
	host := u.Hostname()
	p := u.EscapedPath()

	if isHTML {
		if p == "" || strings.HasSuffix(p, "/") {
			p = path.Join(p, "index.html")
		} else if !strings.Contains(path.Base(p), ".") {
			p = path.Join(p, "index.html")
		}
	}

	out := path.Join(host, p)
	out = strings.TrimPrefix(out, "/")
	return out
}
