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

	uStr := u.String()
	if _, ok := d.visited[uStr]; ok {
		return nil
	}
	d.visited[uStr] = struct{}{}

	log.Printf("[%d] downloading %s", depth, uStr)

	resp, err := d.client.Get(uStr)
	if err != nil {
		log.Printf("[%d] ERROR: request failed for %s: %v", depth, uStr, err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Printf("[%d] ERROR: bad status %d for %s", depth, resp.StatusCode, uStr)
		return fmt.Errorf("bad status %d for %s", resp.StatusCode, uStr)
	}

	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(resp.Body)
		if err == nil {
			defer gz.Close()
			reader = gz
		} else {
			log.Printf("[%d] warning: gzip decode failed for %s: %v", depth, uStr, err)
		}
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		return err
	}
	raw := buf.Bytes()

	ct := resp.Header.Get("Content-Type")
	mediatype, _, _ := mime.ParseMediaType(ct)
	isHTML := mediatype == "text/html"

	relPath := urlutil.CleanPathForFile(u, isHTML)
	d.urlToLocal[uStr] = relPath

	if isHTML {
		htmlStr := string(raw)
		if !strings.Contains(htmlStr, "<base") && strings.Contains(htmlStr, "<head>") {
			htmlStr = strings.Replace(htmlStr, "<head>", "<head>\n<base href=\"./\">", 1)
		}

		links, resources, _, err := parser.ExtractLinksAndResources(u, []byte(htmlStr))
		if err != nil {
			log.Printf("[%d] parse error on %s: %v", depth, uStr, err)
			return err
		}

		if _, err := storage.SaveFile(d.cfg.OutDir, relPath, strings.NewReader(htmlStr)); err != nil {
			return err
		}

		log.Printf("[%d] → found %d links, %d resources in %s", depth, len(links), len(resources), uStr)

		for _, r := range resources {
			ru, err := url.Parse(r)
			if err != nil {
				continue
			}
			if !urlutil.SameDomain(root, ru) {
				continue
			}
			if _, ok := d.visited[ru.String()]; ok {
				continue
			}

			log.Printf("[%d] downloading resource %s", depth, ru)
			if err := d.downloadRecursive(ru, root, depth+1); err != nil {
				log.Printf("[%d] resource fetch failed for %s: %v", depth, ru, err)
			}
		}

		for _, l := range links {
			lu, err := url.Parse(l)
			if err != nil {
				continue
			}
			if !urlutil.SameDomain(root, lu) {
				continue
			}
			if _, ok := d.visited[lu.String()]; ok {
				continue
			}

			log.Printf("[%d] following link %s", depth, lu)
			if err := d.downloadRecursive(lu, root, depth+1); err != nil {
				log.Printf("[%d] link fetch failed for %s: %v", depth, lu, err)
			}
		}

	} else {
		if _, err := storage.SaveFile(d.cfg.OutDir, relPath, bytes.NewReader(raw)); err != nil {
			return err
		}
		log.Printf("[%d] saved file %s (%s)", depth, uStr, relPath)
	}

	return nil
}

func (d *Downloader) rewriteAll() error {
	hrefRe := regexp.MustCompile(`(?i)(href|src)=["']([^"']+)["']`)

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

		s = hrefRe.ReplaceAllStringFunc(s, func(match string) string {
			m := hrefRe.FindStringSubmatch(match)
			if len(m) < 3 {
				return match
			}
			attr, urlPart := m[1], m[2]

			baseURL, _ := url.Parse(uStr)
			targetURL, err := url.Parse(urlPart)
			if err != nil {
				return match
			}
			fullURL := baseURL.ResolveReference(targetURL).String()

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
