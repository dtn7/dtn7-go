// SPDX-FileCopyrightText: 2020 Alvar Penning
//
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/hex"
	"io"
	"io/ioutil"
	"os"

	"github.com/dtn7/dtn7-go/pkg/bpv7"
)

// createBundle for the "create" CLI option.
func createBundle(args []string) {
	if len(args) != 3 && len(args) != 4 {
		printUsage()
	}

	var (
		sender    = args[0]
		receiver  = args[1]
		dataInput = args[2]
		outName   = ""

		err  error
		data []byte
		b    bpv7.Bundle
		f    io.WriteCloser
	)

	if dataInput == "-" {
		data, err = ioutil.ReadAll(os.Stdin)
	} else {
		data, err = ioutil.ReadFile(dataInput)
	}
	if err != nil {
		printFatal(err, "Reading input erred")
	}

	b, err = bpv7.Builder().
		CRC(bpv7.CRC32).
		Source(sender).
		Destination(receiver).
		CreationTimestampNow().
		Lifetime("24h").
		HopCountBlock(64).
		PayloadBlock(data).
		Build()
	if err != nil {
		printFatal(err, "Building Bundle erred")
	}

	if len(args) == 4 {
		outName = args[3]
	} else if outName == "" {
		outName = hex.EncodeToString([]byte(b.ID().String()))
	}

	if outName == "-" {
		f = os.Stdout
	} else if f, err = os.Create(outName); err != nil {
		printFatal(err, "Creating file erred")
	}

	if err = b.MarshalCbor(f); err != nil {
		printFatal(err, "Writing Bundle erred")
	}
	if err = f.Close(); err != nil {
		printFatal(err, "Closing file erred")
	}
}
