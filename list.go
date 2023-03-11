package main

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/emersion/go-smtp"
	"os"
	"path/filepath"
	"strings"
)

type List struct {
	Backend *Backend
	Domain  string
	Name    string
	Type    string
	Ext     string // In case of bounce: The extension in local part
	Status  string
}

func (be *Backend) NewList(e string) (*List, error) {
	c := be.Config
	e = canonicEmail(e)

	a := strings.SplitN(e, "@", 2)
	local, domain := a[0], a[1]

	var found bool
	if domain == c.Domain {
		found = true
	} else if c.Domains != nil {
		for _, d := range c.Domains {
			if domain == d {
				found = true
				break
			}
		}
	}
	if !found {
		return nil, &smtp.SMTPError{
			Code:         554,
			EnhancedCode: smtp.EnhancedCode{5, 7, 1},
			Message:      "Relay access denied",
		}
	}

	var listName, listType, ext string
	if local == c.Email+"-request" {
		listType = "sympaowner"
	} else if local == c.Email {
		listType = "sympa"
	} else if local == c.ListmasterEmail {
		listType = "listmaster"
	} else if strings.HasPrefix(local, c.BounceEmailPrefix+"+") {
		listType = "return_path"
		ext = local[len(c.BounceEmailPrefix+"+"):]
	} else if strings.HasSuffix(local, c.ReturnPathSuffix) &&
		local != c.ReturnPathSuffix {
		listName = local[:len(local)-len(c.ReturnPathSuffix)]
		listType = "return_path"
	} else {
		for _, s := range c.ListCheckSuffixes {
			if strings.HasSuffix(local, "-"+s) &&
				local != "-"+s {
				listName = local[:len(local)-len("-"+s)]
				switch s {
				case "request":
					listType = "owner"
				case "editor":
					listType = "editor"
				case "subscribe":
					listType = "subscribe"
				case "unsubscribe":
					listType = "unsubscribe"
				default:
					listType = "?"
				}
			}
		}
		if listType == "" {
			listName = local
		}
	}
	if listType == "?" {
		return nil, &smtp.SMTPError{
			Code:         550,
			EnhancedCode: smtp.EnhancedCode{5, 1, 1},
			Message:      "Recipient address rejected: User unknown",
		}
	}

	l := &List{
		Backend: be,
		Domain:  domain,
		Name:    listName,
		Type:    listType,
		Ext:     ext,
	}

	if l.Name != "" {
		if err := l.Load(); err != nil {
			return nil, &smtp.SMTPError{
				Code:         550,
				EnhancedCode: smtp.EnhancedCode{5, 1, 1},
				Message:      "Recipient address rejected: User unknown",
			}
		}
	}

	return l, nil
}

func (l *List) Load() error {
	c := l.Backend.Config

	var dir string
	if isDir(filepath.Join(c.Home, l.Domain)) {
		dir = filepath.Join(c.Home, l.Domain, l.Name)
	} else if l.Domain == c.Domain {
		dir = filepath.Join(c.Home, l.Name)
	} else {
		return errors.New(fmt.Sprintf("No such domain %s", l.Domain))
	}

	configpath := filepath.Join(dir, "config")
	file, err := os.Open(configpath)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var para string
	for scanner.Scan() {
		t := strings.TrimSpace(scanner.Text())
		if t == "" {
			para = ""
			continue
		} else if para == "" {
			if len(t) > 7 &&
				strings.ToLower(t[:6]) == "status" &&
				(t[6:7] == " " || t[6:7] == "\t") {
				l.Status = strings.TrimSpace(t[6:])
				break
			}

			para = t
			continue
		} else {
			para = para + "\n" + t
		}
	}

	return nil
}

func (l *List) String() string {
	c := l.Backend.Config

	var s string
	switch l.Type {
	case "sympaowner":
		s = fmt.Sprintf("%s-request@%s", c.Email, l.Domain)
	case "sympa":
		s = fmt.Sprintf("%s@%s", c.Email, l.Domain)
	case "listmaster":
		s = fmt.Sprintf("%s@%s", c.ListmasterEmail, l.Domain)
	case "owner":
		s = fmt.Sprintf("%s-request@%s", l.Name, l.Domain)
	case "editor":
		s = fmt.Sprintf("%s-editor@%s", l.Name, l.Domain)
	case "subscribe":
		s = fmt.Sprintf("%s-subscribe@%s", l.Name, l.Domain)
	case "unsubscribe":
		s = fmt.Sprintf("%s-unsubscribe@%s", l.Name, l.Domain)
	case "return_path":
		if l.Name == "" {
			s = fmt.Sprintf("%s+%s@%s",
				c.BounceEmailPrefix, l.Ext, l.Domain)
		} else {
			s = fmt.Sprintf("%s%s@%s",
				l.Name, c.ReturnPathSuffix, l.Domain)
		}
	case "":
		s = fmt.Sprintf("%s@%s", l.Name, l.Domain)
	}

	return s
}
