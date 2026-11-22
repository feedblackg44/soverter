// Package ipc implements single-instance for `run`: the first process owns a
// loopback port; later processes send their job to it and exit.
package ipc

import (
	"bufio"
	"encoding/json"
	"net"
	"time"
)

const Addr = "127.0.0.1:47613"

// Msg is a job request.
type Msg struct {
	Preset string   `json:"preset"`
	Files  []string `json:"files"`
}

// Listen tries to become the owner. A non-nil error means another instance holds the port.
func Listen() (net.Listener, error) { return net.Listen("tcp", Addr) }

// Send hands a job to the owner.
func Send(m Msg) error {
	c, err := net.DialTimeout("tcp", Addr, 2*time.Second)
	if err != nil {
		return err
	}
	defer c.Close()
	b, _ := json.Marshal(m)
	_, err = c.Write(append(b, '\n'))
	return err
}

// Serve accepts messages until the listener closes.
func Serve(l net.Listener, handle func(Msg)) {
	for {
		c, err := l.Accept()
		if err != nil {
			return
		}
		go func() {
			defer c.Close()
			c.SetReadDeadline(time.Now().Add(5 * time.Second))
			line, err := bufio.NewReader(c).ReadBytes('\n')
			if err != nil {
				return
			}
			var m Msg
			if json.Unmarshal(line, &m) == nil && m.Preset != "" {
				handle(m)
			}
		}()
	}
}
