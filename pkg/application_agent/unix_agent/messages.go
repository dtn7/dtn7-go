// SPDX-FileCopyrightText: 2025 Markus Sommer
//
// SPDX-License-Identifier: GPL-3.0-or-later

package unix_agent

type MessageType uint8

const (
	MsgTypeGeneralResponse   MessageType = 1
	MsgTypeRegisterEID       MessageType = 2
	MsgTypeUnregisterEID     MessageType = 3
	MsgTypeBundleCreate      MessageType = 4
	MsgTypeList              MessageType = 5
	MsgTypeListResponse      MessageType = 6
	MsgTypeGetBundle         MessageType = 7
	MsgTypeGetBundleResponse MessageType = 8
)

type Message struct {
	Type MessageType
}

type GeneralResponse struct {
	Message
	Success bool
	Error   string
}

type RegisterUnregisterMessage struct {
	Message
	EndpointID string
}

type BundleCreateMessage struct {
	Message
	Args map[string]interface{}
}

type MailboxListMessage struct {
	Message
	Mailbox string
	New     bool
}

type MailboxListResponse struct {
	GeneralResponse
	Bundles []string
}

type GetBundleMessage struct {
	Message
	Mailbox  string
	BundleID string
	Remove   bool
}

type BundleContents struct {
	BundleID      string
	SourceID      string
	DestinationID string
	Payload       []byte
}

type GetBundleResponse struct {
	GeneralResponse
	BundleContents
}
