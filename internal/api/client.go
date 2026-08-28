package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"mail-bridge-desktop/internal/config"
	"mail-bridge-desktop/internal/logger"
	"mail-bridge-desktop/internal/store"
)

const (
	requestTimeout  = 30 * time.Second
	clientVersion   = "v1.0.0"
	maxIdleConns    = 20
	idleConnTimeout = 90 * time.Second
)

type service struct {
	baseURL string
	client  string // internxt-client header
}

func (s *service) headers() http.Header {
	h := http.Header{}
	h.Set("Accept", "application/json, text/plain, */*")
	h.Set("internxt-client", s.client)
	h.Set("internxt-version", clientVersion)
	return h
}

type Client struct {
	drive *service
	mail  *service
	http  *http.Client
	log   *logger.Logger
}

func New(cfg config.Config, log *logger.Logger, store *store.Store) (*Client, error) {
	if cfg.MailAPI == "" {
		return nil, errors.New("api: MAIL_API_URL is not set")
	}

	return &Client{
		mail: &service{baseURL: strings.TrimSuffix(cfg.MailAPI, "/"), client: "mail-web"},
		http: &http.Client{
			Timeout: requestTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        maxIdleConns,
				MaxIdleConnsPerHost: maxIdleConns,
				IdleConnTimeout:     idleConnTimeout,
			},
		},
		log: log,
	}, nil
}
