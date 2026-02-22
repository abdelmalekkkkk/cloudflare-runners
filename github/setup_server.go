package github

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
)

//go:embed templates
var templatesFS embed.FS

var initPageTemplate, _ = template.ParseFS(templatesFS, "templates/init.html")
var donePageTemplate, _ = templatesFS.ReadFile("templates/done.html")
var manifestTemplate, _ = template.ParseFS(templatesFS, "templates/manifest.json")

type callback struct {
	code  string
	state string
}

type setupServer struct {
	ctx        context.Context
	listener   net.Listener
	server     http.Server
	callbackCh chan *callback
	errCh      chan error

	params templateParams
}

type templateParams struct {
	organization string
	webhookURL   string
	state        string
}

func CreateSetupServer(ctx context.Context, params templateParams) *setupServer {
	return &setupServer{
		ctx:        ctx,
		params:     params,
		callbackCh: make(chan *callback, 1),
		errCh:      make(chan error, 1),
	}
}

func (s *setupServer) url() (string, error) {
	if s.listener == nil {
		return "", errors.New("listener has not been created yet")
	}

	return fmt.Sprintf("http://localhost:%d", s.listener.Addr().(*net.TCPAddr).Port), nil
}

func (s *setupServer) stop() error {
	return s.server.Shutdown(s.ctx)
}

func (s *setupServer) waitForCallback() (*callback, error) {
	for {
		select {
		case <-s.ctx.Done():
			return nil, errors.Join(s.ctx.Err(), s.stop())
		case callback := <-s.callbackCh:
			return callback, nil
		case err := <-s.errCh:
			return nil, errors.Join(err, s.stop())
		}
	}
}

func (s *setupServer) handleHome(w http.ResponseWriter, r *http.Request) {
	url, err := s.url()

	if err != nil {
		s.errCh <- err
		return
	}

	manifestBuf := &bytes.Buffer{}

	type ManifestParams struct {
		WebhookURL  string
		RedirectURL string
	}

	err = manifestTemplate.Execute(manifestBuf, ManifestParams{
		RedirectURL: fmt.Sprintf("%s/callback", url),
		WebhookURL:  s.params.webhookURL,
	})

	if err != nil {
		s.errCh <- err
		return
	}

	type HomepageParams struct {
		Organization string
		State        string
		ManifestJSON string
	}

	err = initPageTemplate.Execute(w, HomepageParams{
		Organization: s.params.organization,
		State:        s.params.state,
		ManifestJSON: manifestBuf.String(),
	})

	if err != nil {
		s.errCh <- err
		return
	}
}

func (s *setupServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	w.Write(donePageTemplate)

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	s.callbackCh <- &callback{
		code:  code,
		state: state,
	}
}

func (s *setupServer) run() (err error) {
	mux := http.NewServeMux()

	s.listener, err = net.Listen("tcp", ":0")
	if err != nil {
		return err
	}

	s.server = http.Server{
		Addr:    ":0",
		Handler: mux,
	}

	mux.HandleFunc("/", s.handleHome)
	mux.HandleFunc("/callback", s.handleCallback)

	go func() {
		if err := s.server.Serve(s.listener); err != nil {
			s.errCh <- err
		}
	}()
	return
}
