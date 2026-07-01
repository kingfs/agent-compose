package daemon

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/lmittmann/tint"
	"github.com/samber/do/v2"
	"google.golang.org/grpc/codes"

	"agent-compose/pkg/fxgo/echofn"
	"agent-compose/pkg/fxgo/restful"
	"agent-compose/pkg/fxgo/utils"

	"agent-compose/pkg/agentcompose"
	"agent-compose/pkg/auth"
	"agent-compose/pkg/config"
	"agent-compose/pkg/health"
)

type Options struct {
	LoadDotEnv      bool
	SetRlimit       bool
	StartBackground func(do.Injector) error
}

type App struct {
	DI              do.Injector
	Echo            *echo.Echo
	Logger          *slog.Logger
	Config          *config.Config
	startBackground func(do.Injector) error
	startOnce       sync.Once
	startErr        error
}

type daemonServer struct {
	name     string
	value    string
	listener net.Listener
	server   *http.Server
	cleanup  func() error
}

type localUnixSocketRequestKey struct{}

type daemonServers struct {
	items []*daemonServer
}

func NewEcho(di do.Injector) (*echo.Echo, error) {
	e := echo.New()
	e.HTTPErrorHandler = echofn.EchoHTTPErrorHandler
	e.JSONSerializer = echofn.NewEpochTimeJSONSerializer()
	conf := do.MustInvoke[*config.Config](di)

	e.GET("/api/version", func(c echo.Context) error {
		return c.JSON(http.StatusOK, restful.NewResponse[map[string]any, restful.StrStatusResp[map[string]any]](nil, codes.OK.String(), map[string]any{
			"version":   conf.Version,
			"timestamp": float64(time.Now().UnixNano()) / 1e9,
		}))
	})
	e.GET("/api/null", echofn.EchoWrap(restful.NullHandler[restful.StrStatusResp[any]]))
	return e, nil
}

func NewLogger(di do.Injector) (*slog.Logger, error) {
	logger := slog.New(tint.NewHandler(os.Stderr, &tint.Options{NoColor: false, AddSource: true, TimeFormat: "2006-01-02_15:04:05.000"}))
	slog.SetDefault(logger)
	return logger, nil
}

func NewApp(ctx context.Context, opts Options) (*App, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.LoadDotEnv {
		if err := godotenv.Load(); err != nil {
			log.Printf("dotenv load skipped: %v", err)
		}
	}
	if opts.SetRlimit {
		if err := utils.SetRlimitNoFile(); err != nil {
			log.Printf("Warning: Failed to set RLIMIT_NOFILE: %v", err)
		}
	}

	di := do.New()
	do.ProvideValue(di, ctx)
	do.Provide(di, NewLogger)
	config.Setup(di)
	do.Provide(di, NewEcho)
	health.Setup(di)
	agentcompose.Register(di)

	app := do.MustInvoke[*echo.Echo](di)
	logger := do.MustInvoke[*slog.Logger](di)
	conf := do.MustInvoke[*config.Config](di)
	installDaemonMiddleware(app, conf)

	startBackground := opts.StartBackground
	if startBackground == nil {
		startBackground = agentcompose.StartBackground
	}
	return &App{
		DI:              di,
		Echo:            app,
		Logger:          logger,
		Config:          conf,
		startBackground: startBackground,
	}, nil
}

func installDaemonMiddleware(app *echo.Echo, conf *config.Config) {
	app.Use(middleware.RequestLogger())
	app.Use(middleware.Recover())
	authManager := auth.NewAuthManager(&auth.Config{
		AuthUsername:          conf.AuthUsername,
		AuthPassword:          conf.AuthPassword,
		AuthSecret:            conf.AuthSecret,
		AuthSessionTTL:        conf.AuthSessionTTL,
		OAuthAPIKey:           conf.OAuthAPIKey,
		OAuthSecret:           conf.OAuthSecret,
		OAuthScopes:           conf.OAuthScopes,
		OAuthCallbackURL:      conf.OAuthCallbackURL,
		OAuthAuthURL:          conf.OAuthAuthURL,
		OAuthTokenURL:         conf.OAuthTokenURL,
		OAuthUserInfoURL:      conf.OAuthUserInfoURL,
		OAuthClientAuthMethod: conf.OAuthClientAuthMethod,
		Bypass:                isLocalUnixSocketRequest,
		Skipper:               agentcompose.IsRuntimeLLMFacadeRequest,
	})
	authManager.RegisterRoutes(app)
	app.Use(authManager.Middleware)

	if conf.HTTPBasicAuth != "" {
		username := conf.HTTPBasicAuth
		password := ""
		if i := strings.Index(conf.HTTPBasicAuth, ":"); i >= 0 {
			username = conf.HTTPBasicAuth[:i]
			password = conf.HTTPBasicAuth[i+1:]
		}
		app.Use(middleware.BasicAuthWithConfig(middleware.BasicAuthConfig{
			// Same local-trust rule as AuthManager: CLI requests over the Unix
			// socket skip basic auth too.
			Skipper: func(c echo.Context) bool {
				return isLocalUnixSocketRequest(c.Request()) || agentcompose.IsRuntimeLLMFacadeRequest(c.Request())
			},
			Realm: "Password Required",
			Validator: func(u, p string, c echo.Context) (bool, error) {
				return u == username && p == password, nil
			},
		}))
	}
}

func (a *App) StartBackground() error {
	a.startOnce.Do(func() {
		a.startErr = a.startBackground(a.DI)
	})
	return a.startErr
}

func (a *App) Run(ctx context.Context) error {
	servers, err := a.listen()
	if err != nil {
		return err
	}

	if err := a.StartBackground(); err != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if shutdownErr := servers.shutdown(shutdownCtx); shutdownErr != nil {
			err = errors.Join(err, shutdownErr)
		}
		return err
	}

	serverErrCh := servers.serve(a.Logger)
	select {
	case err := <-serverErrCh:
		if err != nil {
			a.Logger.Error("agent-compose server failed", "error", err)
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if shutdownErr := servers.shutdown(shutdownCtx); shutdownErr != nil {
				err = errors.Join(err, shutdownErr)
			}
			return err
		}
	case <-ctx.Done():
		a.Logger.Info("shutdown requested", "error", ctx.Err())
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := servers.shutdown(shutdownCtx); err != nil {
		a.Logger.Error("failed to shutdown agent-compose server", "error", err)
		return err
	}
	return nil
}

func (a *App) listen() (*daemonServers, error) {
	servers := &daemonServers{}

	unixListener, err := listenUnixSocket(a.Config.AgentComposeSocket)
	if err != nil {
		return nil, err
	}
	servers.add("AGENT_COMPOSE_SOCKET", a.Config.AgentComposeSocket, unixListener, a.Echo, func() error {
		return os.Remove(a.Config.AgentComposeSocket)
	})

	if a.Config.HttpListen != "" {
		tcpListener, err := net.Listen("tcp", a.Config.HttpListen)
		if err != nil {
			shutdownErr := servers.shutdown(context.Background())
			return nil, errors.Join(fmt.Errorf("listen HTTP_LISTEN %q: %w", a.Config.HttpListen, err), shutdownErr)
		}
		servers.add("HTTP_LISTEN", a.Config.HttpListen, tcpListener, a.Echo, nil)
	}

	return servers, nil
}

func (s *daemonServers) add(name, value string, listener net.Listener, handler http.Handler, cleanup func() error) {
	server := &http.Server{Handler: handler}
	if listener.Addr().Network() == "unix" {
		server.ConnContext = func(ctx context.Context, conn net.Conn) context.Context {
			if isTrustedUnixSocketConn(conn) {
				return context.WithValue(ctx, localUnixSocketRequestKey{}, true)
			}
			return ctx
		}
	}
	s.items = append(s.items, &daemonServer{
		name:     name,
		value:    value,
		listener: listener,
		server:   server,
		cleanup:  cleanup,
	})
}

// isTrustedUnixSocketConn anchors the auth bypass on the peer's identity
// (SO_PEERCRED / LOCAL_PEERCRED) rather than on socket file permissions alone:
// only the daemon's own user and root get CLI access without credentials.
// Unknown peers (lookup failure, unsupported platform) fall back to normal
// authentication.
func isTrustedUnixSocketConn(conn net.Conn) bool {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return false
	}
	uid, err := unixSocketPeerUID(unixConn)
	if err != nil {
		return false
	}
	return uid == os.Getuid() || uid == 0
}

func isLocalUnixSocketRequest(r *http.Request) bool {
	ok, _ := r.Context().Value(localUnixSocketRequestKey{}).(bool)
	return ok
}

func (s *daemonServers) serve(logger *slog.Logger) <-chan error {
	errCh := make(chan error, len(s.items))
	for _, item := range s.items {
		go func(item *daemonServer) {
			logger.Info("agent-compose listener started", "config", item.name, "addr", item.listener.Addr().String())
			err := item.server.Serve(item.listener)
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- fmt.Errorf("serve %s %q: %w", item.name, item.value, err)
				return
			}
			errCh <- nil
		}(item)
	}
	return errCh
}

func (s *daemonServers) shutdown(ctx context.Context) error {
	var joined error
	for _, item := range s.items {
		if err := item.server.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			joined = errors.Join(joined, fmt.Errorf("shutdown %s %q: %w", item.name, item.value, err))
		}
		if err := item.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			joined = errors.Join(joined, fmt.Errorf("close %s %q: %w", item.name, item.value, err))
		}
		if item.cleanup != nil {
			if err := item.cleanup(); err != nil && !errors.Is(err, os.ErrNotExist) {
				joined = errors.Join(joined, fmt.Errorf("cleanup %s %q: %w", item.name, item.value, err))
			}
		}
	}
	return joined
}

func listenUnixSocket(path string) (net.Listener, error) {
	if path == "" {
		return nil, fmt.Errorf("AGENT_COMPOSE_SOCKET is empty")
	}
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return nil, fmt.Errorf("prepare AGENT_COMPOSE_SOCKET %q: create parent %q: %w", path, parent, err)
	}

	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("prepare AGENT_COMPOSE_SOCKET %q: path exists and is not a Unix socket", path)
		}
		if err := removeStaleUnixSocket(path); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("prepare AGENT_COMPOSE_SOCKET %q: stat socket path: %w", path, err)
	}

	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen AGENT_COMPOSE_SOCKET %q: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		closeErr := listener.Close()
		removeErr := os.Remove(path)
		return nil, errors.Join(fmt.Errorf("prepare AGENT_COMPOSE_SOCKET %q: chmod socket: %w", path, err), closeErr, removeErr)
	}
	return listener, nil
}

func removeStaleUnixSocket(path string) error {
	conn, err := net.DialTimeout("unix", path, 100*time.Millisecond)
	if err == nil {
		if closeErr := conn.Close(); closeErr != nil {
			return fmt.Errorf("prepare AGENT_COMPOSE_SOCKET %q: close active socket probe: %w", path, closeErr)
		}
		return fmt.Errorf("prepare AGENT_COMPOSE_SOCKET %q: socket already in use", path)
	}
	if os.IsPermission(err) {
		return fmt.Errorf("prepare AGENT_COMPOSE_SOCKET %q: probe socket: %w", path, err)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("prepare AGENT_COMPOSE_SOCKET %q: remove stale socket: %w", path, err)
	}
	return nil
}

func Run(ctx context.Context) error {
	app, err := NewApp(ctx, Options{LoadDotEnv: true, SetRlimit: true})
	if err != nil {
		return err
	}
	return app.Run(ctx)
}
