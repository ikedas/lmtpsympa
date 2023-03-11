package main

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"os"
	"regexp"
	"strings"
)

const (
	domainRe   = "[-a-z0-9]+(?:[.][-a-z0-9]+)+"
	emailRe    = "(?:[-A-Za-z0-9!#$%&'*+/=?^_`{|}~.]+|\"(?:\\.|[^\\\"])*\")@[-A-Za-z0-9]+(?:[.][-A-Za-z0-9]+)+"
	listnameRe = "[a-z0-9][-a-z0-9.+_]*"
)

func addrToIP(hostport string) (net.IP, error) {
	h, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return nil, err
	}
	ip := net.ParseIP(h)
	if ip == nil {
		return nil, errors.New("Illegal syntax")
	}
	return ip, nil
}

func addressLiteral(ip net.IP) string {
	if ip4 := ip.To4(); ip4 != nil {
		return fmt.Sprintf("[%s]", ip4)
	} else {
		return fmt.Sprintf("[IPv6:%s]", ip.String())
	}
}

func canonicDomain(d string) string {
	return toLowerASCII(d)
}

func canonicEmail(e string) string {
	return toLowerASCII(strings.TrimSpace(e))
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	if err != nil {
		return false
	}
	return fi.IsDir()
}

func toLowerASCII(s string) string {
	return strings.Map(
		func(r rune) rune {
			if 'A' <= r && r <= 'Z' {
				return r + ('a' - 'A')
			}
			return r
		}, s,
	)
}

func uniqueId() string {
	return fmt.Sprintf("%010d", rand.Intn((2<<31)-1)+2)
}

func validateDomain(d string) error {
	d = canonicDomain(d)

	matched, err := regexp.Match("^"+domainRe+"$", []byte(d))
	if err != nil {
		panic(err)
	}
	if matched {
		return nil
	}
	return errors.New("invalid domain name")
}

func validateEmail(e string) error {
	if e == "" {
		return nil
	}
	matched, err := regexp.Match("^"+emailRe+"$", []byte(e))
	if err != nil {
		panic(err)
	}
	if matched {
		return nil
	}
	return errors.New("invalid email address")
}
