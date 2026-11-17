package service

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	nethtml "golang.org/x/net/html"
)

// Preserve script order and attributes (including modules) while moving inline
// executable content into same-origin assets allowed by Telemt's decoy CSP.
// Raw owner HTML used by the ordinary domain gateway is not changed.
func telemtWebImportedPageFiles(page string) map[string]string {
	files := map[string]string{}
	var output strings.Builder
	tokenizer := nethtml.NewTokenizer(strings.NewReader(page))
	sequence := 0
	for {
		kind := tokenizer.Next()
		if kind == nethtml.ErrorToken {
			break
		}
		raw := string(tokenizer.Raw())
		if kind != nethtml.StartTagToken {
			output.WriteString(raw)
			continue
		}
		token := tokenizer.Token()
		extract := token.Data == "style"
		if token.Data == "script" {
			extract = true
			for _, attr := range token.Attr {
				if attr.Key == "src" {
					extract = false
				}
				if attr.Key == "type" {
					switch strings.ToLower(strings.TrimSpace(attr.Val)) {
					case "", "text/javascript", "application/javascript", "text/ecmascript", "application/ecmascript", "module":
					default:
						extract = false
					}
				}
			}
		}
		if !extract {
			output.WriteString(raw)
			continue
		}
		var content strings.Builder
		for {
			kind = tokenizer.Next()
			if kind == nethtml.ErrorToken {
				break
			}
			nextRaw := string(tokenizer.Raw())
			if kind == nethtml.EndTagToken && tokenizer.Token().Data == token.Data {
				break
			}
			content.WriteString(nextRaw)
		}
		sequence++
		extension := "js"
		if token.Data == "style" {
			extension = "css"
		}
		name := fmt.Sprintf("site-inline-%d.%s", sequence, extension)
		files[name] = content.String()
		if token.Data == "style" {
			link := nethtml.Token{Type: nethtml.StartTagToken, Data: "link", Attr: []nethtml.Attribute{{Key: "rel", Val: "stylesheet"}, {Key: "href", Val: "/" + name}}}
			for _, attr := range token.Attr {
				if attr.Key == "media" || attr.Key == "title" || attr.Key == "id" {
					link.Attr = append(link.Attr, attr)
				}
			}
			output.WriteString(link.String())
		} else {
			token.Attr = append(token.Attr, nethtml.Attribute{Key: "src", Val: "/" + name})
			output.WriteString(token.String())
			output.WriteString("</script>")
		}
	}
	files["index.html"] = output.String()
	return files
}

func telemtWebImportedSnapshotPath(sitesDir, page string) string {
	data, _ := json.Marshal(telemtWebImportedPageFiles(page))
	return filepath.Join(sitesDir, fmt.Sprintf("%x", sha256.Sum256(data)))
}
