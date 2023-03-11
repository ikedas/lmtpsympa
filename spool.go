package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

type Spool struct {
	Dir string
}

func (be *Backend) NewSpoolIncoming() (*Spool, error) {
	c := be.Config

	if !isDir(c.QueueIncoming) {
		return nil, errors.New("No directory")
	}
	return &Spool{Dir: c.QueueIncoming}, nil
}

func (be *Backend) NewSpoolBounce() (*Spool, error) {
	c := be.Config

	if !isDir(c.QueueBounce) {
		return nil, errors.New("No directory")
	}
	return &Spool{Dir: c.QueueBounce}, nil
}

func (spool *Spool) Store(l *List, m *Message) error {
	var a string
	if l.Type == "return_path" {
		if l.Name == "" {
			a = l.Backend.Config.Email + "@" + l.Domain
		} else {
			a = l.Name + "@" + l.Domain
		}
	} else {
		a = l.String()
	}

	fn := fmt.Sprintf("%s.%d.%s", a, time.Now().Unix(), m.SessionId)
	tmppath := filepath.Join(spool.Dir, "T."+fn)
	path := filepath.Join(spool.Dir, fn)
	file, err :=
		os.OpenFile(tmppath, syscall.O_CREAT|syscall.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := file.Write(m.Serialized()); err != nil {
		return err
	}
	if err := os.Rename(tmppath, path); err != nil {
		return err
	}

	return nil
}
