// SPDX-FileCopyrightText: 2025 Markus Sommer
//
// SPDX-License-Identifier: GPL-3.0-or-later

package unix_agent

type MessageType uint8

const (
	MsgTypeBundleCreate         MessageType = 1
	MsgTypeBundleCreateResponse MessageType = 2
)

type Message struct {
	Type MessageType
}

type BundleCreate struct {
	Message
	Source      string
	Destination string
	Payload     []byte
}

type BundleCreateResponse struct {
	Message
	Success bool
	Error   string
}
