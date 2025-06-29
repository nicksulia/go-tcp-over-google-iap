package iap

const (
	// IAPHostURL is the base URL for the Identity-Aware Proxy (IAP) tunnel service.
	// Source: iap-desktop
	IAPHostURL        = "tunnel.cloudproxy.app"
	WebSocketProtocol = "wss"
	ConnectPath       = "/v4/connect"
	ReconnectPath     = "/v4/reconnect"
	// RelayProtocolName is the protocol name used for the SSH Relay v4.
	RelayProtocolName = "relay.tunnel.cloudproxy.app"
	Origin            = "bot:iap-tunneler"
	UserAgent         = "iap-tunneler-cli/1.0"
	// Message tags used by SSH Relay v4. Source: iap-desktop/sources/Google.Solutions.Iap/Protocol/SshRelayConstants.cs
	// Big-endian encoding is used for the message tags.
	RelayConnectSuccessSID   uint16 = 0x0001
	RelayReconnectSuccessACK uint16 = 0x0002
	RelayData                uint16 = 0x0004
	RelayACK                 uint16 = 0x0007

	MessageTagLen = 2 // Length of the message tag in bytes (big-endian encoding, 2 bytes for uint16
	//
	MessageLen           = 4                          // Length of the message length field in bytes (big-endian encoding, 4 bytes for uint32
	DataMessageHeaderLen = MessageTagLen + MessageLen // Length of the data message header in bytes (2 bytes for tag + 4 bytes for length)
	ACKLen               = 8                          // Length of the ACK message in bytes 64-bit unsigned integer (8 bytes for uint64)
	SIDLen               = 4                          // Length of the SID in bytes (32-bit unsigned integer, 4 bytes for uint32)
	SIDHeaderLen         = MessageTagLen + SIDLen     // Length of the SID message header in bytes (2 bytes for tag + 4 bytes for length)
	ACKHeaderLen         = MessageTagLen + ACKLen     // Length of the ACK message header in bytes (2 bytes for tag + 8 bytes for length)
	MaxMessageSize       = 1024 * 16                  // Maximum default message size in bytes (16 KB). Defined by the protocol specification.

	CloseStatusNormal          = 1000
	CloseStatusAbnormalClosure = 1006
	// Custom statuses
	CloseStatusErrorUnknown             = 4000
	CloseStatusSIDUnknown               = 4001
	CloseStatusSIDInUse                 = 4002
	CloseStatusFailedToConnectToBackend = 4003 // Instance is offline
	CloseStatusReauthenticationRequired = 4004
	CloseStatusBadACK                   = 4005
	CloseStatusInvalidACK               = 4006
	CloseStatusInvalidSocketOpcode      = 4007
	CloseStatusInvalidTag               = 4008
	CloseStatusDestinationWriteFailed   = 4009
	CloseStatusDestinationReadFailed    = 4010 // Error might appear during SSH, RDP, WinRM sessions on Exit
	CloseStatusInvalidData              = 4013
	CloseStatusNotAuthorized            = 4033
	CloseStatusLookupFailed             = 4047
	CloseStatusLookupFailedReconnect    = 4051
	CloseStatusFailedToRewind           = 4074
)
