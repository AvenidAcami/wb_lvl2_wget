package downloader

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"wb_lvl2_wget/internal/parser"
	"wb_lvl2_wget/internal/storage"
	"wb_lvl2_wget/internal/urlutil"
)

type Config struct {
	OutDir   string
	MaxDepth int
	Timeout  time.Duration
}

type Downloader struct {
	cfg        Config
	client     *http.Client
	visited    map[string]struct{}
	urlToLocal map[string]string
}

func New(cfg Config) (*Downloader, error) {
	return &Downloader{
		cfg:        cfg,
		client:     &http.Client{Timeout: cfg.Timeout},
		visited:    make(map[string]struct{}),
		urlToLocal: make(map[string]string),
	}, nil
}

func (d *Downloader) Run(startURL string) error {
	root, err := url.Parse(startURL)
	if err != nil {
		return err
	}

	log.Printf("Starting mirror of %s (depth=%d)", startURL, d.cfg.MaxDepth)
	if err := d.downloadRecursive(root, root, 0); err != nil {
		return err
	}

	if err := d.rewriteAll(); err != nil {
		log.Printf("rewriteAll error: %v", err)
	}
	return nil
}

func (d *Downloader) downloadRecursive(u, root *url.URL, depth int) error {
	if depth > d.cfg.MaxDepth {
		return nil
	}
	if _, ok := d.visited[u.String()]; ok {
		return nil
	}
	d.visited[u.String()] = struct{}{}

	log.Printf("[%d] downloading %s", depth, u.String())
	resp, err := d.client.Get(u.String())
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("bad status %d for %s", resp.StatusCode, u.String())
	}

	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(resp.Body)
		if err == nil {
			defer gz.Close()
			reader = gz
		} else {
			log.Printf("warning: gzip decode failed for %s: %v", u.String(), err)
		}
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		return err
	}
	rawHTML := buf.Bytes()

	ct := resp.Header.Get("Content-Type")
	mediatype, _, _ := mime.ParseMediaType(ct)
	relPath := urlutil.CleanPathForFile(u)

	if mediatype == "text/html" || strings.HasSuffix(u.Path, "/") {
		htmlStr := string(rawHTML)

		replacements := []struct {
			old string
			new string
		}{
			{`src="/`, `src="./`},
			{`href="/`, `href="./`},
			{`action="/`, `action="./`},
			{`url("/`, `url("./`},
		}
		for _, r := range replacements {
			htmlStr = strings.ReplaceAll(htmlStr, r.old, r.new)
		}

		if !strings.Contains(htmlStr, "<base") && strings.Contains(htmlStr, "<head>") {
			htmlStr = strings.Replace(htmlStr, "<head>", "<head>\n<base href=\"./\">", 1)
		}

		if _, err := storage.SaveFile(d.cfg.OutDir, relPath, strings.NewReader(htmlStr)); err != nil {
			return err
		}

		links, resources, _, err := parser.ExtractLinksAndResources(u, []byte(htmlStr))
		if err != nil {
			return err
		}

		d.urlToLocal[u.String()] = relPath

		for _, r := range resources {
			ru, err := url.Parse(r)
			if err == nil && urlutil.SameDomain(root, ru) {
				_ = d.downloadRecursive(ru, root, depth+1)
			}
		}

		for _, l := range links {
			lu, err := url.Parse(l)
			if err == nil && urlutil.SameDomain(root, lu) {
				_ = d.downloadRecursive(lu, root, depth+1)
			}
		}
	} else {
		if _, err := storage.SaveFile(d.cfg.OutDir, relPath, bytes.NewReader(rawHTML)); err != nil {
			return err
		}
		d.urlToLocal[u.String()] = relPath
	}

	return nil
}

func (d *Downloader) rewriteAll() error {
	hrefRe := regexp.MustCompile(`(?i)(href|src)=["'](/[^"']+)["']`)

	for uStr, local := range d.urlToLocal {
		if !(strings.HasSuffix(local, ".html") || strings.HasSuffix(local, "index.html")) {
			continue
		}

		fullPath := path.Join(d.cfg.OutDir, local)
		b, err := os.ReadFile(fullPath)
		if err != nil {
			return err
		}

		s := string(b)
		baseURL, _ := url.Parse(uStr)

		for srcURL, localPath := range d.urlToLocal {
			targetURL, _ := url.Parse(srcURL)
			if !urlutil.SameDomain(baseURL, targetURL) {
				continue
			}

			rel := relativeFilePath(local, localPath)
			s = strings.ReplaceAll(s, srcURL, rel)
		}

		s = hrefRe.ReplaceAllStringFunc(s, func(match string) string {
			m := hrefRe.FindStringSubmatch(match)
			if len(m) < 3 {
				return match
			}
			attr, absPath := m[1], m[2]

			fakeBase, _ := url.Parse(uStr)
			target, _ := url.Parse(absPath)
			fullURL := fakeBase.ResolveReference(target).String()

			localPath, ok := d.urlToLocal[fullURL]
			if !ok {
				return match
			}

			rel := relativeFilePath(local, localPath)
			return fmt.Sprintf(`%s="%s"`, attr, rel)
		})

		if err := os.WriteFile(fullPath, []byte(s), 0o644); err != nil {
			return err
		}
	}

	return nil
}

func relativeFilePath(from, to string) string {
	fromDir := filepath.Dir(from)
	rel, err := filepath.Rel(fromDir, to)
	if err != nil {
		return to
	}
	return filepath.ToSlash(rel)
}
