package beachplug

import (
	"net/http"

	"fmt"
	"github.com/vincent-petithory/kraken/fileserver"
	"html/template"
	"log"
	"net/url"
	"sort"
	"time"
)

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
}
var tpl = template.Must(template.New("").Funcs(fm).Parse(tplstr))

var tplstr = fmt.Sprintf(`<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
    <title>Index on {{.Root}}</title>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style type="text/css">%s</style>
  </head>
<body>
  <h3>{{.Root}}</h3>
  <hr/>
  <table>
  {{range sorted .Directories}}
    <tr>
      <td><a href="{{ urlpath . }}/">{{ . }}/</a></td>
    </tr>
  {{end}}
  {{ if and .NumDirectories .NumFiles }}
  <tr><td colspan="3"><hr/></td></tr>
  {{ end }}

  <tr>
    <th>File</th>
    <th>Size</th>
    <th>Mod time</th>
  </tr>
  {{range sorted .Files}}
    <tr>
      <td><a href="{{ urlpath .Name }}">{{ .Name }}</a></td>
      <td>{{ .Size }}</td>
      <td>{{ .ModTime }}</td>
    </tr>
  {{end}}
  </table>
  <hr/>
</body>
</html>
`, css)

const css = `
html {
  color: #222;
  font-size: 10px;
  font-family: Monospace;
}

hr {
  display: block;
  height: 1px;
  border: 0;
  border-top: 1px solid #ccc;
  margin: 1em 0;
  padding: 0;
}

th {
  font-weight: bold;
  text-align: left;
}
`
