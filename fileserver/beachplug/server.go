package beachplug

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/vincent-petithory/kraken/fileserver"
)

// Server defines the beachplug server constructor.
var Server fileserver.Constructor = func(root string, params fileserver.Params) fileserver.Server {
	return &server{
		root: root,
		fs:   http.Dir(root),
	}
}

type server struct {
	fs   http.FileSystem
	root string
}

func (s server) Root() string {
	return s.root
}

func (s server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path[0] != '/' {
		r.URL.Path = "/" + r.URL.Path
	}
	f, err := s.fs.Open(r.URL.Path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if !fi.IsDir() {
		http.ServeContent(w, r, fi.Name(), fi.ModTime(), f)
		return
	}
	if r.URL.Path[len(r.URL.Path)-1] != '/' {
		w.Header().Set("Location", r.URL.Path+"/")
		w.WriteHeader(http.StatusMovedPermanently)
		return
	}

	// Dir listing
	ctx := tplCtx{
		Root:        r.URL.Path,
		Style:       css,
		Files:       make(filelist, 0),
		Directories: make(dirlist, 0),
	}
	for {
		fis, err := f.Readdir(100)
		if err != nil || len(fis) == 0 {
			break
		}
		for _, fi := range fis {
			if fi.IsDir() {
				ctx.Directories = append(ctx.Directories, fi.Name())
			} else {
				f := file{fi.Name(), fi.Size(), fi.ModTime().Truncate(time.Second)}
				ctx.Files = append(ctx.Files, f)
			}
		}
	}
	if r.URL.Path != "/" {
		ctx.Directories = append(dirlist{".."}, ctx.Directories...)
	}
	ctx.NumFiles = len(ctx.Files)
	ctx.NumDirectories = len(ctx.Directories)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := tpl.Execute(w, ctx); err != nil {
		log.Print(err)
	}
}

type tplCtx struct {
	Root           string
	Style          template.CSS
	Files          filelist
	Directories    dirlist
	NumFiles       int
	NumDirectories int
}

type file struct {
	Name    string
	Size    int64
	ModTime time.Time
}

type dirlist []string

func (l dirlist) Less(i int, j int) bool {
	return l[i] < l[j]
}
func (l dirlist) Swap(i int, j int) { l[i], l[j] = l[j], l[i] }
func (l dirlist) Len() int          { return len(l) }

type filelist []file

func (l filelist) Less(i int, j int) bool {
	if l[i].Name == l[j].Name {
		return l[i].ModTime.Before(l[j].ModTime)
	}
	return l[i].Name < l[j].Name
}
func (l filelist) Swap(i int, j int) { l[i], l[j] = l[j], l[i] }
func (l filelist) Len() int          { return len(l) }

const (
	kib = 1024
	mib = 1024 * 1024
	gib = 1024 * 1024 * 1024
)

var fm = map[string]interface{}{
	"urlpath": func(path string) string {
		return (&url.URL{Path: path}).String()
	},
	"sorted": func(v interface{}) interface{} {
		if sv, ok := v.(sort.Interface); ok {
			sort.Sort(sv)
		}
		return v
	},
	"fmttime": func(t time.Time) string {
		return t.Format("2006-01-02 15:04:05")
	},
	"humanbytes": func(n int64) string {
		switch {
		case n > gib:
			return fmt.Sprintf("%.1f GiB", float64(n)/gib)
		case n > mib:
			return fmt.Sprintf("%.1f MiB", float64(n)/mib)
		case n > kib:
			return fmt.Sprintf("%.1f KiB", float64(n)/kib)
		default:
			return fmt.Sprintf("%d B", n)
		}
	},
	"ellipsis": func(s string, max int) string {
		if utf8.RuneCountInString(s) > max {
			return fmt.Sprintf("%."+fmt.Sprintf("%d", max-1)+"s…", s)
		}
		return s
	},
}

var tpl = template.Must(template.New("").Funcs(fm).Parse(tplstr))

var tplstr = `<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
    <title>Index on {{.Root}}</title>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style type="text/css">{{.Style}}</style>
  </head>
<body>
  <div class="header">
  <a href="https://github.com/vincent-petithory/kraken">kraken</a>
  <h3>{{.Root}}</h3> 
  </div>
  <div class="contents">
    <table>
    {{range sorted .Directories}}
      <tr>
        <td colspan="3"><a href="{{ urlpath . }}/">{{ . }}/</a></td>
      </tr>
    {{end}}
    </table>
    {{ if and .NumDirectories .NumFiles }}
    <hr/>
    {{ end }}

    {{ if .NumFiles }}
    <table>
    <tr>
      <th>File</th>
      <th>Size</th>
      <th>Mod time</th>
    </tr>
    {{range sorted .Files}}
      <tr>
        <td><a href="{{ urlpath .Name }}">{{ ellipsis .Name 50 }}</a></td>
        <td>{{ humanbytes .Size }}</td>
        <td>{{ fmttime .ModTime }}</td>
      </tr>
    {{end}}
    </table>
    {{end}}
  </div>
</body>
</html>
`

const css = template.CSS(`
* {
  padding: 0;
  margin: 0;
}

html {
  color: rgb(55, 55, 55);
  font-family: Sans;
}

body {
  background: rgb(255, 233, 198);
}

hr {
  display: block;
  border: 0;
  border-top: 3px solid rgb(199, 65, 79);
  margin: 1em 0;
  padding: 0;
}

th {
  font-weight: bold;
  text-align: left;
}

th, td {
  padding: 2px;
}

.header {
  background: rgb(199, 65, 79);
  padding: 22px 15px;
}
.header a, .header a:visited {
  float: right;
  color: rgb(55, 55, 55);
  font-size: 1.2em;
  font-weight: bold;
  transition: color 0.2s;
  text-decoration: none;
}
.header a:hover {
  color: rgb(75, 75, 75);
  transition: color 0.2s;
}

.contents {
  margin-top: 20px;
  margin-left: 20%;
  margin-right: 20%;
  font-size: 0.7em;
}

.contents a, .contents a:visited {
  color: rgb(199, 65, 79);
  padding: 2px;
  background: transparent;
  transition: background 0.3s, color 0.2s;
  text-decoration: none;
}
.contents a:hover {
  background: rgb(199, 65, 79);
  color: rgb(255, 233, 198);
  transition: background 0.2s, color 0.2s;
}
`)
