package main

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/goccy/go-yaml"
	"golang.org/x/net/netutil"
)

type Config_service struct {
	E_ bool

	Addr              string `yaml:"addr"`
	LMTP              bool
	ESMTP_            bool   `yaml:"esmtp"`
	Domain            string `yaml:"iam"`
	MaxRecipients     int    `yaml:"max_rcpt"`
	MaxMessageBytes   int64  `yaml:"max_size"`
	MaxLineLength     int
	AllowInsecureAuth bool `yaml:"allow_insecure_auth"`
	ReadTimeout       time.Duration
	ReadTimeout_      uint `yaml:"read_timeout"`
	WriteTimeout      time.Duration
	WriteTimeout_     uint `yaml:"write_timeout"`

	EnableSMTPUTF8   bool
	EnableREQUIRETLS bool
	EnableBINARYMIME bool
	AuthDisabled     bool

	MaxConnections int    `yaml:"max_connections"`
	Username       string `yaml:"user"`
	Groupname      string `yaml:"group"`
	SocketMode     int
	SocketMode_    string `yaml:"mode"`
	FileUmask      int
	FileUmask_     string `yaml:"umask"`
}

type Config_custom struct {
	E_ bool

	EOL         string `yaml:"eol"`
	AddReceived bool   `yaml:"add_received"`
}

type Config struct {
	Domain            string   `yaml:"domain"`
	Domains           []string `yaml:"domains"`
	Home              string   `yaml:"home"`
	QueueIncoming     string   `yaml:"queue"`
	QueueBounce       string   `yaml:"queuebounce"`
	QueueAutomatic    string   `yaml:"queueautomatic"`
	BounceEmailPrefix string   `yaml:"bounce_email_prefix"`
	ReturnPathSuffix  string   `yaml:"return_path_suffix"`
	ListCheckSuffixes []string `yaml:"list_check_suffixes"`
	Email             string   `yaml:"email"`
	ListmasterEmail   string   `yaml:"listmaster_email"`

	S Config_service `yaml:"service"`
	C Config_custom  `yaml:"custom"`
}

func NewConfig() (*Config, error) {
	config := Config{
		BounceEmailPrefix: "bounce",
		ReturnPathSuffix:  "-owner",
		ListCheckSuffixes: []string{
			"request", "owner", "editor",
			"unsubscribe", "subscribe",
		},
		Email:           "sympa",
		ListmasterEmail: "listmaster",
	}

	config.S.E_ = true
	config.S.Domain = "localhost"
	config.S.MaxConnections = 100
	config.S.MaxRecipients = 50
	config.S.MaxMessageBytes = 5 * 1024 * 1024
	config.S.SocketMode_ = "666"
	config.S.FileUmask_ = "027"
	config.S.ReadTimeout_ = 300
	config.S.WriteTimeout_ = 300

	config.C.E_ = true
	config.C.EOL = "\n"
	config.C.AddReceived = true

	var file *os.File
	var err error
	buf := make([]byte, 8192)
	if len(os.Args) > 1 {
		file, err = os.Open(os.Args[1])
		if err != nil {
			return nil, err
		}
		defer file.Close()
	} else {
		file = os.Stdin
	}
	_, err = file.Read(buf)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal([]byte(buf), &config); err != nil {
		return nil, err
	}
	if !config.S.E_ || !config.C.E_ {
		return nil, errors.New("empty configuration")
	}

	if config.S.FileUmask_ != "" {
		mode64, err := strconv.ParseUint(config.S.FileUmask_, 8, 9)
		if err != nil {
			return nil, err
		}
		config.S.FileUmask = int(mode64)
	}
	if config.S.ESMTP_ {
		// TODO: Handle "subsequent failure" with (E)SMTP
		config.S.MaxRecipients = 1
	} else {
		config.S.LMTP = true
	}
	if config.S.SocketMode_ != "" {
		mode64, err := strconv.ParseUint(config.S.SocketMode_, 8, 9)
		if err != nil {
			return nil, err
		}
		config.S.SocketMode = int(mode64)
	}
	if config.S.ReadTimeout_ > 0 {
		config.S.ReadTimeout =
			time.Duration(config.S.ReadTimeout_) * time.Second
	}
	if config.S.WriteTimeout_ > 0 {
		config.S.WriteTimeout =
			time.Duration(config.S.WriteTimeout_) * time.Second
	}
	config.S.AuthDisabled = true

	return &config, nil
}

// Server is the smtp.Server extended
type Server struct {
	*smtp.Server
	MaxConnections int
	Username       string
	Groupname      string
	SocketMode     int
	FileUmask      int
}

func NewServer(be *Backend) *Server {
	s := &Server{smtp.NewServer(be), 0, "", "", 0, 0}

	s.Addr = be.Config.S.Addr
	s.LMTP = be.Config.S.LMTP
	s.Domain = be.Config.S.Domain
	s.ReadTimeout = be.Config.S.ReadTimeout
	s.WriteTimeout = be.Config.S.WriteTimeout
	s.MaxMessageBytes = be.Config.S.MaxMessageBytes
	s.MaxRecipients = be.Config.S.MaxRecipients
	s.AllowInsecureAuth = be.Config.S.AllowInsecureAuth
	s.AuthDisabled = be.Config.S.AuthDisabled

	s.MaxConnections = be.Config.S.MaxConnections
	s.Username = be.Config.S.Username
	s.Groupname = be.Config.S.Groupname
	s.SocketMode = be.Config.S.SocketMode
	s.FileUmask = be.Config.S.FileUmask

	return s
}

func (s *Server) ListenAndServe() error {
	return listenAndServe(s, false)
}

func (s *Server) ListenAndServeTLS() error {
	return listenAndServe(s, true)
}

func listenAndServe(s *Server, useTLS bool) error {
	addr := s.Addr
	if addr == "" {
		if s.LMTP {
			addr = ":24"
		} else {
			addr = ":smtp"
		}
	}

	network := "tcp"
	if strings.Contains(addr, "/") {
		network = "unix"
	}

	var (
		l   net.Listener
		err error
	)
	if network == "unix" && s.SocketMode > 0 {
		mode := s.SocketMode
		umask := syscall.Umask(-1 &^ mode)
		l, err = net.Listen(network, addr)
		syscall.Umask(umask)
		if err != nil {
			return err
		}
		if err = os.Chmod(addr, os.FileMode(mode)); err != nil {
			return err
		}
	} else {
		l, err = net.Listen(network, addr)
		if err != nil {
			return err
		}
	}
	if s.MaxConnections > 0 {
		l = netutil.LimitListener(l, s.MaxConnections)
	}
	if useTLS {
		l = tls.NewListener(l, s.TLSConfig)
	}

	if s.Username != "" {
		var userU *user.User

		var uid int
		_, err = strconv.Atoi(s.Username)
		if err == nil {
			userU, err = user.LookupId(s.Username)
		}
		if err != nil {
			userU, err = user.Lookup(s.Username)
		}
		if err != nil {
			return err
		}
		uid, err = strconv.Atoi(userU.Uid)
		if err != nil {
			return err
		}

		var gids []int
		if os.Getuid() == 0 {
			groups, err := userU.GroupIds()
			if err != nil {
				return err
			}
			for _, g := range groups {
				id, err := strconv.Atoi(g)
				if err != nil {
					return err
				}
				gids = append(gids, id)
			}
		}

		var gid int
		if s.Groupname != "" {
			var userG *user.Group

			_, err = strconv.Atoi(s.Groupname)
			if err == nil {
				userG, err = user.LookupGroupId(s.Groupname)
			}
			if err != nil {
				userG, err = user.LookupGroup(s.Groupname)
			}
			if err != nil {
				return err
			}
			gid, err = strconv.Atoi(userG.Gid)
		} else {
			gid, err = strconv.Atoi(userU.Gid)
		}
		if err != nil {
			return err
		}

		if network == "unix" {
			if err := os.Chown(addr, uid, gid); err != nil {
				return err
			}
		}
		if err := syscall.Setgid(gid); err != nil {
			return err
		}
		if gids != nil {
			if err := syscall.Setgroups(gids); err != nil {
				return err
			}
		}
		if err := syscall.Setuid(uid); err != nil {
			return err
		}
	}

	if s.FileUmask > 0 {
		syscall.Umask(s.FileUmask)
	}

	if os.Getuid() == 0 {
		os.Stderr.WriteString("*** You are running as root.")
		os.Stderr.WriteString(
			" Running as an unprivileged user is recommended.\n")
	}

	rand.Seed(time.Now().UnixNano())

	return s.Serve(l)
}

// The Backend implements SMTP server methods.
type Backend struct {
	Config *Config
}

func NewBackend(c *Config) *Backend {
	return &Backend{Config: c}
}

func (be *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	id := uniqueId()
	ra := c.Conn().RemoteAddr().String()
	la := c.Conn().LocalAddr().String()
	hh := c.Hostname()
	log.Printf("%s client=<%s>, hello=<%s>, bound=<%s>", id, ra, hh, la)

	if be.Config.S.LMTP {
		return &LMTPSession{Session{
			Id:         id,
			Backend:    be,
			RemoteAddr: ra,
			LocalAddr:  la,
			HelloHost:  canonicDomain(hh),
		}}, nil
	} else {
		return &Session{
			Id:         id,
			Backend:    be,
			RemoteAddr: ra,
			LocalAddr:  la,
			HelloHost:  canonicDomain(hh),
		}, nil
	}
}

// A Session is returned after EHLO.
type Session struct {
	Id         string
	Backend    *Backend
	RemoteAddr string
	LocalAddr  string
	HelloHost  string
	MailFrom   string
	RcptTos    []string
	RcptLists  []*List
	DataErrors []error
}

type LMTPSession struct {
	Session
}

func (s *Session) AuthPlain(username, password string) error {
	if username != "username" || password != "password" {
		return errors.New("Invalid username or password")
	}
	return nil
}

func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	log.Printf("%s from=<%s>", s.Id, from)

	if from == "" {
		return nil
	}

	if err := validateEmail(from); err != nil {
		return &smtp.SMTPError{
			Code:         501,
			EnhancedCode: smtp.EnhancedCode{5, 1, 7},
			Message:      "Bad sender address syntax",
		}
	}
	s.MailFrom = from
	return nil
}

func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	log.Printf("%s to=<%s>", s.Id, to)

	if err := validateEmail(to); err != nil {
		return &smtp.SMTPError{
			Code:         501,
			EnhancedCode: smtp.EnhancedCode{5, 1, 3},
			Message:      "Bad recipient address syntax",
		}
	}

	l, err := s.Backend.NewList(to)
	if err != nil {
		return err
	}
	if l.Name != "" && l.Status != "open" {
		return &smtp.SMTPError{
			Code:         550,
			EnhancedCode: smtp.EnhancedCode{5, 1, 1},
			Message:      "Recipient address rejected: User unknown ",
		}
	}

	s.RcptTos = append(s.RcptTos, to)
	s.RcptLists = append(s.RcptLists, l)
	return nil
}

func (s *Session) storeData(r io.Reader) error {
	var message *Message
	if b, err := ioutil.ReadAll(r); err != nil {
		return err
	} else {
		log.Printf("%s data(%d bytes)", s.Id, len(b))

		message = s.Backend.NewMessage(s, b)
	}

	ispool, err := s.Backend.NewSpoolIncoming()
	if err != nil {
		return err
	}
	bspool, err := s.Backend.NewSpoolBounce()
	if err != nil {
		return err
	}

	sent := make(map[string]error)
	for _, l := range s.RcptLists {
		a := l.String()

		var err error
		if _, ok := sent[a]; ok {
			err = sent[a]
		} else {
			if l.Type == "return_path" {
				err = bspool.Store(l, message)
			} else {
				err = ispool.Store(l, message)
			}
			sent[a] = err
			log.Printf("%s to=<%s>, error=<%v>", s.Id, l, err)
		}
		s.DataErrors = append(s.DataErrors, err)
	}

	return nil
}

func (s *Session) Data(r io.Reader) error {
	if err := s.storeData(r); err != nil {
		return err
	}

	var succ, fail bool
	for _, err := range s.DataErrors {
		if err == nil {
			succ = true
		} else {
			fail = true
		}
	}
	if succ {
		if fail {
			// FIXME: handle partial errors
		}
		return nil
	} else {
		return errors.New("Internal error")
	}
}

func (s *Session) LMTPData(r io.Reader, status smtp.StatusCollector) error {
	if err := s.storeData(r); err != nil {
		return err
	}
	for i, err := range s.DataErrors {
		status.SetStatus(s.RcptTos[i], err)
	}

	return nil
}

func (s *Session) Reset() {
	log.Printf("%s rset", s.Id)

	s.Id = uniqueId()

	s.MailFrom = ""
	s.RcptTos = nil
	s.RcptLists = nil
	s.DataErrors = nil
}

func (s *Session) Logout() error {
	log.Printf("%s quit", s.Id)

	return nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()

	c, err := NewConfig()
	if err != nil {
		log.Fatal(err)
	}
	be := NewBackend(c)
	s := NewServer(be)

	go func() {
		<-ctx.Done()
		ctx, cancel := context.WithTimeout(context.Background(),
			60*time.Second)
		defer cancel()
		s.Shutdown(ctx)
	}()

	log.Println("Starting server at", s.Addr)
	if err := s.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
