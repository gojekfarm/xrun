package component

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/gojekfarm/xrun"
)

type HTTPServerSuite struct {
	suite.Suite
}

func TestHTTPServerSuite(t *testing.T) {
	suite.Run(t, new(HTTPServerSuite))
}

func (s *HTTPServerSuite) TestHTTPServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("pong"))
	})

	testcases := []struct {
		name                string
		server              *http.Server
		testFunc            func(s *suite.Suite) func() bool
		wantErr             bool
		wantShutdownTimeout bool
	}{
		{
			name: "SuccessfulStart",
			server: &http.Server{
				Addr:    ":8888",
				Handler: mux,
			},
			testFunc: func(s *suite.Suite) func() bool {
				return func() bool {
					resp, err := http.Get("http://localhost:8888/ping")
					s.NoError(err)
					defer func() {
						s.NoError(resp.Body.Close())
					}()
					if d, err := io.ReadAll(resp.Body); err == nil {
						return string(d) == "pong"
					}
					return false
				}
			},
		},
		{
			name: "FailedStart",
			server: &http.Server{
				Addr:    ":-9090",
				Handler: mux,
			},
			testFunc: func(s *suite.Suite) func() bool {
				i := 0
				return func() bool {
					time.Sleep(100 * time.Millisecond)
					i++
					return i > 3
				}
			},
			wantErr: true,
		},
		{
			name: "UnlimitedShutdownWait",
			server: &http.Server{
				Addr:    ":9999",
				Handler: mux,
			},
		},
		{
			name: "ShutdownTimeout",
			server: &http.Server{
				Addr:    ":9999",
				Handler: mux,
			},
			wantShutdownTimeout: true,
			wantErr:             true,
		},
	}

	for _, t := range testcases {
		s.Run(t.name, func() {
			var opts []xrun.Option
			if t.wantShutdownTimeout {
				opts = append(opts, xrun.ShutdownTimeout(time.Nanosecond))
			}

			m := xrun.NewManager(opts...)
			st := s.T()

			s.NoError(m.Add(HTTPServer(
				HTTPServerOptions{
					Server:   t.server,
					PreStart: func() { st.Log("PreStart called") },
					PreStop:  func() { st.Log("PreStop called") },
				},
			)))

			errCh := make(chan error, 1)
			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				errCh <- m.Run(ctx)
			}()

			time.Sleep(50 * time.Millisecond)

			if t.testFunc != nil {
				s.Eventually(t.testFunc(&s.Suite), 10*time.Second, 100*time.Millisecond)
			}

			cancel()
			if t.wantErr {
				s.Error(<-errCh)
			} else {
				s.NoError(<-errCh)
			}

			time.Sleep(50 * time.Millisecond) // for goroutine to exit
		})
	}
}
