package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"syscall"
	"text/template"
	"time"

	"github.com/cptaffe/blog/accept-headers"
)

var linksPath = flag.String("links", "", "path to links file")
var templatesPath = flag.String("templates", "", "path to templates")
var address = flag.String("address", ":8080", "address to listen on, :8080 by default")

type Link struct {
	Short string `json:"short"`
	Long  string `json:"long"`
}

type Links struct {
	Links  []Link
	lookup map[string]*Link
}

func (l *Links) Get(short string) (*Link, bool) {
	if l.lookup == nil {
		l.buildLookup()
	}
	link, ok := l.lookup[short]
	return link, ok
}

func (l *Links) buildLookup() {
	l.lookup = make(map[string]*Link)
	for i := range l.Links {
		link := &l.Links[i]
		l.lookup[link.Short] = link
	}
}

type Server struct {
	links     *Links
	mux       *http.ServeMux
	templates *template.Template
}

func NewServer(links *Links, templates *template.Template) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /l/", &Server{links: links, templates: templates})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Add("Content-Type", "text/plain; charset=UTF-8")
		fmt.Fprint(w, "ok")
	})
	return mux
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.mux == nil {
		s.mux = http.NewServeMux()
		s.mux.HandleFunc("GET /l/{short}", s.Elongate)
		s.mux.HandleFunc("GET /l/", s.Index)
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) Elongate(w http.ResponseWriter, r *http.Request) {
	headers := w.Header()
	headers.Add("Content-Type", "text/html; charset=UTF-8")
	headers.Add("Cache-Control", "private; max-age=90")
	headers.Add("Referrer-Policy", "unsafe-url")

	link, ok := s.links.Get(r.PathValue("short"))
	if !ok {
		http.NotFound(w, r)
		return
	}

	http.Redirect(w, r, link.Long, http.StatusMovedPermanently)
}

func (s *Server) Index(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Cache-Control", "private; max-age=90")
	s.WriteRenderedTemplate(w, accept.Parse(r.Header.Values("Accept")), "index.html.tmpl", s.links)
}

func (s *Server) WriteRenderedTemplate(w http.ResponseWriter, options accept.AcceptSlice, template string, data any) {
	if options.AcceptsByName("application/xhtml+xml") {
		w.Header().Add("Content-Type", "application/xhtml+xml; charset=UTF-8")
	} else {
		// default to text/html
		w.Header().Add("Content-Type", "text/html; charset=UTF-8")
		fmt.Println("<!DOCTYPE html>")
	}
	err := s.templates.ExecuteTemplate(w, template, data)
	if err != nil {
		log.Println("render page as html", err)
		http.Error(w, "failed to render page", http.StatusInternalServerError)
		return
	}
}

func main() {
	flag.Parse()

	f, err := os.Open(*linksPath)
	if err != nil {
		log.Fatalf("open links file `%s`: %v", *linksPath, err)
	}
	var links Links
	err = json.NewDecoder(f).Decode(&links)
	if err != nil {
		log.Fatalf("decode links file `%s`: %v", *linksPath, err)
	}

	templates, err := template.New("html").ParseGlob(path.Join(*templatesPath, "*.html.tmpl"))
	if err != nil {
		log.Fatalf("parse html templates `%s`: %w", templates, err)
	}

	server := http.Server{Addr: *address, Handler: NewServer(&links, templates)}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	log.Println("starting server")
	go func() {
		err = server.ListenAndServe()
		switch err {
		case nil, http.ErrServerClosed:
		default:
			log.Fatal("listen and serve", err)
		}
	}()

	<-done
	log.Println("stopping server")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = server.Shutdown(ctx)
	if err != nil {
		log.Fatalf("shutdown server: %v", err)
	}
	log.Println("server stopped")
}
