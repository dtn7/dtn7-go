// SPDX-FileCopyrightText: 2025 Markus Sommer
//
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/akamensky/argparse"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/dtn7/dtn7-go/pkg/application_agent/unix_agent"
)

func main() {
	parser := argparse.NewParser("dtnclient", "Interact with dtnd via the UNIX application agent")
	parser.ExitOnHelp(true)
	socketName := parser.String("s", "socket", &argparse.Options{Help: "UNIX socket", Required: false, Default: "/tmp/dtnd.socket"})

	register := parser.NewCommand("register", "Register EndpointID")
	registerID := register.String("i", "eid", &argparse.Options{Help: "Valid bpv7 EndpointID", Required: true})
	unRegister := parser.NewCommand("unregister", "Unregister EndpointID")
	unRegisterID := unRegister.String("i", "eid", &argparse.Options{Help: "Valid bpv7 EndpointID", Required: true})

	err := parser.Parse(os.Args)
	if err != nil {
		fmt.Print(parser.Usage(err))
		os.Exit(1)
	}

	socketAddr, err := net.ResolveUnixAddr("unix", *socketName)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	conn, err := net.DialUnix("unix", nil, socketAddr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	connReader := bufio.NewReader(conn)
	connWriter := bufio.NewWriter(conn)

	if register.Happened() {
		handleRegisterUnregister(connReader, connWriter, *registerID, true)
	} else if unRegister.Happened() {
		handleRegisterUnregister(connReader, connWriter, *unRegisterID, false)
	}
}

func handleRegisterUnregister(connReader *bufio.Reader, connWriter *bufio.Writer, eid string, register bool) {
	// create message
	msg := unix_agent.RegisterUnregister{
		Message:    unix_agent.Message{},
		EndpointID: eid,
	}
	if register {
		msg.Message.Type = unix_agent.MsgTypeRegisterEID
	} else {
		msg.Message.Type = unix_agent.MsgTypeUnregisterEID
	}

	msgBytes, err := msgpack.Marshal(&msg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	msgLen := uint64(len(msgBytes))
	msgLenBytes := make([]byte, 8)
	_, err = binary.Encode(msgLenBytes, binary.BigEndian, msgLen)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	//send message
	// send length of message
	_, err = connWriter.Write(msgLenBytes)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// send message
	_, err = connWriter.Write(msgBytes)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	err = connWriter.Flush()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// receive reply
	// read length or reply
	_, err = io.ReadFull(connReader, msgLenBytes)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	msgLen = binary.BigEndian.Uint64(msgLenBytes)
	// create buffer of correct length
	msgBytes = make([]byte, msgLen)
	// read reply into buffer
	_, err = io.ReadFull(connReader, msgBytes)
	reply := unix_agent.GeneralResponse{}
	err = msgpack.Unmarshal(msgBytes, &reply)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if !reply.Success {
		fmt.Fprintln(os.Stderr, reply.Error)
		os.Exit(1)
	} else {
		fmt.Println("Success")
		os.Exit(0)
	}
}
