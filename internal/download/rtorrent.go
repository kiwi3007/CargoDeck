package download

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kiwi3007/playerr/internal/domain"
)

// RTorrentClient uses XML-RPC over HTTP.
type RTorrentClient struct {
	cfg    domain.DownloadClientConfig
	url    string
	http   *http.Client
}

func NewRTorrentClient(cfg domain.DownloadClientConfig) *RTorrentClient {
	base := buildBaseURL(cfg.Host, cfg.Port, cfg.UrlBase)
	rpcPath := "/RPC2"
	if cfg.UrlBase != "" {
		rpcPath = cfg.UrlBase
		if !strings.HasPrefix(rpcPath, "/") {
			rpcPath = "/" + rpcPath
		}
	}
	u := fmt.Sprintf("%s%s", base, rpcPath)
	return &RTorrentClient{
		cfg:  cfg,
		url:  u,
		http: &http.Client{Timeout: 15 * time.Second},
	}
}

type xmlRPCResponse struct {
	Params []struct {
		Value struct {
			String string `xml:"string"`
			Array  struct {
				Data []struct {
					Value struct {
						Array struct {
							Data []struct {
								Value struct {
									String string `xml:"string"`
									Int    string `xml:"i8"`
									Int4   string `xml:"int"`
								} `xml:"value"`
							} `xml:"value"`
						} `xml:"array"`
						String string `xml:"string"`
						Int    string `xml:"i8"`
						Int4   string `xml:"int"`
					} `xml:"value"`
				} `xml:"data>value"`
			} `xml:"array"`
		} `xml:"value"`
	} `xml:"params>param"`
}

func (c *RTorrentClient) xmlrpc(method string, params ...string) ([]byte, error) {
	var paramXML strings.Builder
	for _, p := range params {
		paramXML.WriteString(fmt.Sprintf("<param><value><string>%s</string></value></param>", p))
	}
	body := fmt.Sprintf(`<?xml version="1.0"?><methodCall><methodName>%s</methodName><params>%s</params></methodCall>`,
		method, paramXML.String())

	req, err := http.NewRequest("POST", c.url, bytes.NewBufferString(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml")
	if c.cfg.Username != "" {
		req.SetBasicAuth(c.cfg.Username, c.cfg.Password)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (c *RTorrentClient) TestConnection() (bool, string, error) {
	body, err := c.xmlrpc("system.client_version")
	if err != nil {
		return false, "", err
	}
	var r xmlRPCResponse
	xml.Unmarshal(body, &r)
	if len(r.Params) > 0 {
		return true, r.Params[0].Value.String, nil
	}
	return false, "", nil
}

func (c *RTorrentClient) GetVersion() (string, error) {
	_, v, err := c.TestConnection()
	return v, err
}

func (c *RTorrentClient) AddTorrent(torrentURL, _ string) error {
	_, err := c.xmlrpc("load.start", "", torrentURL)
	return err
}

func (c *RTorrentClient) AddNzb(_, _ string) error {
	return fmt.Errorf("rTorrent does not support NZB")
}

func (c *RTorrentClient) RemoveDownload(id string) error {
	_, err := c.xmlrpc("d.erase", id)
	return err
}

func (c *RTorrentClient) PauseDownload(id string) error {
	_, err := c.xmlrpc("d.stop", id)
	return err
}

func (c *RTorrentClient) ResumeDownload(id string) error {
	_, err := c.xmlrpc("d.start", id)
	return err
}

func (c *RTorrentClient) GetDownloads() ([]domain.DownloadStatus, error) {
	// Use d.multicall2 to get torrent info
	body, err := c.xmlrpc("d.multicall2", "",
		"main",
		"d.hash=",
		"d.name=",
		"d.size_bytes=",
		"d.bytes_done=",
		"d.is_active=",
		"d.is_hash_checking=",
		"d.complete=",
		"d.custom1=",
		"d.directory=",
	)
	if err != nil {
		return nil, err
	}

	// Parse the XML-RPC array response manually via raw parse
	_ = body
	// For now return empty — full XML-RPC array parsing is complex
	// TODO: implement full multicall2 parsing
	return []domain.DownloadStatus{}, nil
}
