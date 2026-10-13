package sd_socket

import (
	"encoding/binary"
	"net"

	"golang.org/x/sys/unix"
)

const (
	sizeofSockaddrInet4 = 0x10
	sizeofSockaddrInet6 = 0x1c
)

// Converts a net.UDPAddr to a Sockaddr
// Returns nil if conversion fails
func udpAddrToSockaddr(addr *net.UDPAddr) unix.Sockaddr {
	// undefined IP address, use IPv6 default
	if addr.IP == nil {
		return &unix.SockaddrInet6{
			Port:   addr.Port,
			ZoneId: uint32(ip6ZoneToInt(addr.Zone)),
		}
	}

	// valid IPv4 address
	if ip4 := addr.IP.To4(); ip4 != nil && addr.Zone == "" {
		var buf [4]byte
		copy(buf[:], ip4)
		return &unix.SockaddrInet4{
			Addr: buf,
			Port: addr.Port,
		}
	}

	// valid IPv6 address
	if ip6 := addr.IP.To16(); ip6 != nil {
		var buf [16]byte
		copy(buf[:], ip6)
		return &unix.SockaddrInet6{
			Addr:   buf,
			Port:   addr.Port,
			ZoneId: uint32(ip6ZoneToInt(addr.Zone)),
		}
	}

	return nil
}

// Converts an IP6 Zone net string to a unix int
// returns 0 if zone is ""
func ip6ZoneToInt(zone string) int {
	if zone == "" {
		return 0
	}

	if ifi, err := net.InterfaceByName(zone); err == nil {
		return ifi.Index
	}

	// decimal string to int
	n := 0
	for i := 0; i < len(zone) && '0' <= zone[i] && zone[i] <= '9'; i++ {
		n = n*10 + int(zone[i]-'0')
		// should not bigger than max uint32
		if n > 0xFFFFFFFF {
			return 0
		}
	}
	return n
}

// Converts a unix.Sockaddr to a net.UDPAddr
// Returns nil if conversion fails
func SockaddrToUDPAddr(sa unix.Sockaddr) *net.UDPAddr {
	switch sa := sa.(type) {
	case *unix.SockaddrInet4:
		ip := make(net.IP, net.IPv4len)
		copy(ip, sa.Addr[:])
		return &net.UDPAddr{
			IP:   ip,
			Port: sa.Port,
		}

	case *unix.SockaddrInet6:
		ip := make([]byte, 16)
		copy(ip, sa.Addr[:])
		return &net.UDPAddr{
			IP:   ip,
			Port: sa.Port,
			Zone: ip6ZoneToString(int(sa.ZoneId)),
		}
	}

	return nil
}

// Converts an IP6 Zone unix int to a net string
// returns "" if zone is 0
func ip6ZoneToString(zone int) string {
	if zone == 0 {
		return ""
	}

	if ifi, err := net.InterfaceByIndex(zone); err == nil {
		return ifi.Name
	}

	// int to decimal string
	var b [32]byte
	bp := len(b)
	for ; zone > 0; zone /= 10 {
		bp--
		b[bp] = byte(zone%10) + '0'
	}
	return string(b[bp:])
}

// Resolve a string addr to a Sockaddr
// Returns nil if resolve fails
func ResolveUDPSockaddr(strAddr string) unix.Sockaddr {
	netAddr, err := net.ResolveUDPAddr("udp", strAddr)
	if err != nil {
		return nil
	}

	return udpAddrToSockaddr(netAddr)
}

// Converts a name buffer to a Sockaddr
// Returns nil if conversion fails
func nameBufferToSockaddr(buf []byte, nameLen uint32) unix.Sockaddr {
	if nameLen > uint32(len(buf)) {
		return nil
	}

	switch nameLen {
	case sizeofSockaddrInet4:
		var ip [4]byte
		copy(ip[:], buf[4:8])
		return &unix.SockaddrInet4{
			Addr: ip,
			Port: int(binary.BigEndian.Uint16(buf[2:4])),
		}

	case sizeofSockaddrInet6:
		var ip [16]byte
		copy(ip[:], buf[8:24])
		return &unix.SockaddrInet6{
			Addr:   ip,
			Port:   int(binary.BigEndian.Uint16(buf[2:4])),
			ZoneId: binary.BigEndian.Uint32(buf[24:28]),
		}
	}

	return nil
}

// Converts a Sockaddr to a name buffer
// Returns nil if conversion fails
// Returns the name buffer and the length of the buffer
func sockaddrToNameBuffer(sa unix.Sockaddr) ([]byte, uint32) {
	switch addr := sa.(type) {
	case *unix.SockaddrInet4:
		if addr.Port < 0 || addr.Port > 0xffff {
			return nil, 0
		}

		buf := make([]byte, sizeofSockaddrInet4)
		binary.BigEndian.PutUint16(buf[2:4], uint16(addr.Port))
		copy(buf[4:8], addr.Addr[:])
		return buf, sizeofSockaddrInet4

	case *unix.SockaddrInet6:
		if addr.Port < 0 || addr.Port > 0xffff {
			return nil, 0
		}

		buf := make([]byte, sizeofSockaddrInet6)
		binary.BigEndian.PutUint16(buf[2:4], uint16(addr.Port))
		copy(buf[8:24], addr.Addr[:])
		binary.BigEndian.PutUint32(buf[24:28], addr.ZoneId)
		return buf, sizeofSockaddrInet6
	}

	return nil, 0
}

// Converts a name buffer to a net.UDPAddr
// Returns nil if conversion fails
func nameBufferToUDPAddr(buf []byte, nameLen uint32) *net.UDPAddr {
	if nameLen > uint32(len(buf)) {
		return nil
	}

	switch nameLen {
	case sizeofSockaddrInet4:
		ip := make(net.IP, net.IPv4len)
		copy(ip, buf[4:8])
		return &net.UDPAddr{
			IP:   ip,
			Port: int(binary.BigEndian.Uint16(buf[2:4])),
		}

	case sizeofSockaddrInet6:
		ip := make(net.IP, net.IPv6len)
		copy(ip[:], buf[8:24])
		return &net.UDPAddr{
			IP:   ip,
			Port: int(binary.BigEndian.Uint16(buf[2:4])),
			Zone: ip6ZoneToString(int(binary.BigEndian.Uint32(buf[24:28]))),
		}
	}

	return nil
}

// Converts a net.UDPAddr to a name buffer
// Returns nil if conversion fails
// Returns the name buffer and the length of the buffer
func udpAddrToNameBuffer(addr *net.UDPAddr) ([]byte, uint32) {
	if addr == nil {
		return nil, 0
	}

	return sockaddrToNameBuffer(udpAddrToSockaddr(addr))
}
