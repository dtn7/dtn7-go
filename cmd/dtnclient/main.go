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
	address := parser.String("a", "address", &argparse.Options{
		Help:     "UNIX socket",
		Required: false,
		Default:  "/tmp/dtnd.socket",
	})

	register := parser.NewCommand("register", "Register EndpointID")
	registerID := register.String("i", "eid", &argparse.Options{
		Help:     "Valid bpv7 EndpointID",
		Required: true,
	})
	unRegister := parser.NewCommand("unregister", "Unregister EndpointID")
	unRegisterID := unRegister.String("i", "eid", &argparse.Options{
		Help:     "Valid bpv7 EndpointID",
		Required: true,
	})

	create := parser.NewCommand("create", "Create a bundle")
	sourceID := create.String("s", "source", &argparse.Options{
		Help:     "Bundle source EndpointID",
		Required: true,
	})
	destinationID := create.String("d", "destination", &argparse.Options{
		Help:     "Bundle destination EndpointID",
		Required: true,
	})
	reportTo := create.String("r", "report", &argparse.Options{
		Help:     "EndpointID to send status reports to",
		Required: false,
		Default:  "",
	})
	creationTimestamp := create.String("t", "timestamp", &argparse.Options{
		Help:     "Bundle's creation timestamp",
		Required: false,
		Default:  "now",
	})
	lifetime := create.String("l", "lifetime", &argparse.Options{
		Help:     "Bundle's lifetime",
		Required: false,
		Default:  "24h",
	})
	payload := create.String("p", "payload", &argparse.Options{
		Help:     "Bundle's payload, either a filename to read or 'stdin'",
		Required: false,
		Default:  "stdin",
	})

	list := parser.NewCommand("list", "Query bundles in mailbox")
	listMailboxID := list.String("i", "id", &argparse.Options{
		Help:     "EndpointID of mailbox",
		Required: true,
	})
	listNew := list.Flag("n", "new", &argparse.Options{
		Help:     "List only new bundles (which have not been retrieved)",
		Required: false,
		Default:  false,
	})

	get := parser.NewCommand("get", "Get bundles from mailbox")
	getMailboxID := get.String("m", "mailbox", &argparse.Options{
		Help:     "EndpointID of mailbox",
		Required: true,
	})
	getRemove := get.Flag("r", "remove", &argparse.Options{
		Help:     "Delete bundle from mailbox after retrieval",
		Required: false,
		Default:  false,
	})

	getBundle := get.NewCommand("bundle", "Get bundle by ID")
	getBundleID := getBundle.String("b", "bundle", &argparse.Options{
		Help:     "BundleID of bundle",
		Required: true,
	})
	getBundleOutput := getBundle.String("o", "out", &argparse.Options{
		Help:     "Where to output bundle payload, either 'stdout' or a filename",
		Required: false,
		Default:  "stdout",
	})

	err := parser.Parse(os.Args)
	if err != nil {
		fmt.Print(parser.Usage(err))
		os.Exit(1)
	}

	socketAddr, err := net.ResolveUnixAddr("unix", *address)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	conn, err := net.DialUnix("unix", nil, socketAddr)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	connReader := bufio.NewReader(conn)
	connWriter := bufio.NewWriter(conn)

	if register.Happened() {
		handleRegisterUnregister(connReader, connWriter, *registerID, true)
	} else if unRegister.Happened() {
		handleRegisterUnregister(connReader, connWriter, *unRegisterID, false)
	} else if create.Happened() {
		handleCreate(
			connReader,
			connWriter,
			*sourceID,
			*destinationID,
			*reportTo,
			*creationTimestamp,
			*lifetime,
			*payload,
		)
	} else if list.Happened() {
		handleList(connReader, connWriter, *listMailboxID, *listNew)
	} else if get.Happened() {
		if getBundle.Happened() {
			handleGetBundle(connReader, connWriter, *getMailboxID, *getBundleID, *getBundleOutput, *getRemove)
		}
	}
}

func handleRegisterUnregister(connReader *bufio.Reader, connWriter *bufio.Writer, eid string, register bool) {
	// create message
	msg := unix_agent.RegisterUnregisterMessage{
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
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	msgLen := uint64(len(msgBytes))
	msgLenBytes := make([]byte, 8)
	_, err = binary.Encode(msgLenBytes, binary.BigEndian, msgLen)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	//send message
	// send length of message
	_, err = connWriter.Write(msgLenBytes)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// send message
	_, err = connWriter.Write(msgBytes)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	err = connWriter.Flush()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// receive reply
	// read length or reply
	_, err = io.ReadFull(connReader, msgLenBytes)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
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
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if !reply.Success {
		_, _ = fmt.Fprintln(os.Stderr, reply.Error)
		os.Exit(1)
	} else {
		_, _ = fmt.Println("Success")
		os.Exit(0)
	}
}

func handleCreate(
	connReader *bufio.Reader,
	connWriter *bufio.Writer,
	sourceID, destinationID, reportTo, creationTimestamp, lifetime, payload string,
) {
	var err error
	args := make(map[string]interface{})
	args["source"] = sourceID
	args["destination"] = destinationID
	if reportTo != "" {
		args["report_to"] = reportTo
	}
	if creationTimestamp == "now" {
		args["creation_timestamp_now"] = true
	} else if creationTimestamp == "epoch" {
		args["creation_timestamp_epoch"] = true
	} else {
		args["creation_timestamp_time"] = creationTimestamp
	}
	args["lifetime"] = lifetime

	var payloadBytes []byte
	if payload == "stdin" {
		reader := bufio.NewReader(os.Stdin)
		payload, err = reader.ReadString('\n')
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		payloadBytes = []byte(payload)
	} else {
		file, err := os.Open(payload)
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer file.Close()
		payloadBytes, err = io.ReadAll(file)
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	args["payload_block"] = payloadBytes

	msg := unix_agent.BundleCreateMessage{
		Message: unix_agent.Message{Type: unix_agent.MsgTypeBundleCreate},
		Args:    args,
	}

	msgBytes, err := msgpack.Marshal(&msg)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	msgLen := uint64(len(msgBytes))
	msgLenBytes := make([]byte, 8)
	_, err = binary.Encode(msgLenBytes, binary.BigEndian, msgLen)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	//send message
	// send length of message
	_, err = connWriter.Write(msgLenBytes)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// send message
	_, err = connWriter.Write(msgBytes)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	err = connWriter.Flush()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// receive reply
	// read length or reply
	_, err = io.ReadFull(connReader, msgLenBytes)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
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
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if !reply.Success {
		_, _ = fmt.Fprintln(os.Stderr, reply.Error)
		os.Exit(1)
	} else {
		fmt.Println("Success")
		os.Exit(0)
	}
}

func handleList(connReader *bufio.Reader, connWriter *bufio.Writer, mailboxID string, new bool) {
	msg := unix_agent.MailboxListMessage{
		Message: unix_agent.Message{Type: unix_agent.MsgTypeList},
		Mailbox: mailboxID,
		New:     new,
	}

	msgBytes, err := msgpack.Marshal(&msg)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	msgLen := uint64(len(msgBytes))
	msgLenBytes := make([]byte, 8)
	_, err = binary.Encode(msgLenBytes, binary.BigEndian, msgLen)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	//send message
	// send length of message
	_, err = connWriter.Write(msgLenBytes)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// send message
	_, err = connWriter.Write(msgBytes)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	err = connWriter.Flush()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// receive reply
	// read length or reply
	_, err = io.ReadFull(connReader, msgLenBytes)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	msgLen = binary.BigEndian.Uint64(msgLenBytes)
	// create buffer of correct length
	msgBytes = make([]byte, msgLen)
	// read reply into buffer
	_, err = io.ReadFull(connReader, msgBytes)
	reply := unix_agent.MailboxListResponse{}
	err = msgpack.Unmarshal(msgBytes, &reply)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if !reply.Success {
		_, _ = fmt.Fprintln(os.Stderr, reply.Error)
		os.Exit(1)
	} else {
		_, _ = fmt.Println(reply.Bundles)
		os.Exit(0)
	}
}

func handleGetBundle(connReader *bufio.Reader, connWriter *bufio.Writer, mailboxID, bundleID, output string, remove bool) {
	msg := unix_agent.GetBundleMessage{
		Message:  unix_agent.Message{Type: unix_agent.MsgTypeGetBundle},
		Mailbox:  mailboxID,
		BundleID: bundleID,
		Remove:   remove,
	}

	msgBytes, err := msgpack.Marshal(&msg)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	msgLen := uint64(len(msgBytes))
	msgLenBytes := make([]byte, 8)
	_, err = binary.Encode(msgLenBytes, binary.BigEndian, msgLen)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	//send message
	// send length of message
	_, err = connWriter.Write(msgLenBytes)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// send message
	_, err = connWriter.Write(msgBytes)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	err = connWriter.Flush()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// receive reply
	// read length or reply
	_, err = io.ReadFull(connReader, msgLenBytes)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	msgLen = binary.BigEndian.Uint64(msgLenBytes)
	// create buffer of correct length
	msgBytes = make([]byte, msgLen)
	// read reply into buffer
	_, err = io.ReadFull(connReader, msgBytes)
	reply := unix_agent.GetBundleResponse{}
	err = msgpack.Unmarshal(msgBytes, &reply)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if !reply.Success {
		_, _ = fmt.Fprintln(os.Stderr, reply.Error)
		os.Exit(1)
	} else {
		if output == "stdout" {
			_, _ = fmt.Print(string(reply.Payload))
		} else {
			file, err := os.Create(output)
			if err != nil {
				_, _ = fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			defer file.Close()
			_, err = file.Write(reply.Payload)
			if err != nil {
				_, _ = fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
		os.Exit(0)
	}
}
