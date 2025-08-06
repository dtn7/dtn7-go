// SPDX-FileCopyrightText: 2025 Markus Sommer
//
// SPDX-License-Identifier: GPL-3.0-or-later

package unix_agent

type MessageType uint8

const (
	MsgTypeGeneralResponse MessageType = 1
	MsgTypeRegisterEID     MessageType = 2
	MsgTypeUnregisterEID   MessageType = 3
	MsgTypeBundleCreate    MessageType = 4
)

type Message struct {
	Type MessageType
}

type GeneralResponse struct {
	Message
	Success bool
	Error   string
}

type RegisterUnregister struct {
	Message
	EndpointID string
}

type BundleCreate struct {
	Message
	Args map[string]interface{}
}
