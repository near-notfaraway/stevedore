package sd_socket

import (
	"encoding/binary"
	"golang.org/x/sys/unix"
	"net"
)

const (
	SizeofSockaddrInet4 = 0x10
	SizeofSockaddrInet6 = 0x1c
)

// Converts a net.UDPAddr to a Sockaddr.
// Returns nil if conversion fails.
func UDPAddrToSockaddr(addr *net.UDPAddr) unix.Sockaddr {
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
// Returns nil if conversion fails.
func SockaddrToUDPAddr(sa unix.Sockaddr) *net.UDPAddr {
	switch sa := sa.(type) {
	case *unix.SockaddrInet4:
		ip := make([]byte, 16)
		copy(ip[12:16], sa.Addr[:])
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
// Returns nil if resolve fails.
func ResolveUDPSockaddr(strAddr string) unix.Sockaddr {
	netAddr, err := net.ResolveUDPAddr("udp", strAddr)
	if err != nil {
		return nil
	}

	return UDPAddrToSockaddr(netAddr)
}

// Converts a name buffer to a Sockaddr
// Returns nil if conversion fails.
func NameBufferToSockaddr(buf []byte, len uint32) unix.Sockaddr {
	switch len {
	case SizeofSockaddrInet4:
		var ip [4]byte
		copy(ip[:], buf[4:8])
		return &unix.SockaddrInet4{
			Addr: ip,
			Port: int(binary.BigEndian.Uint16(buf[2:4])),
		}

	case SizeofSockaddrInet6:
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

func NameBufferToUDPAddr(buf []byte, len uint32) *net.UDPAddr {
	switch len {
	case SizeofSockaddrInet4:
		ip := make([]byte, 16)
		copy(ip[12:16], buf[4:8])
		return &net.UDPAddr{
			IP:   ip,
			Port: int(binary.BigEndian.Uint16(buf[2:4])),
		}

	case SizeofSockaddrInet6:
		ip := make([]byte, 16)
		copy(ip[:], buf[8:24])
		return &net.UDPAddr{
			IP:   make([]byte, 16),
			Port: int(binary.BigEndian.Uint16(buf[2:4])),
			Zone: ip6ZoneToString(int(binary.BigEndian.Uint32(buf[24:28]))),
		}
	}

	return nil
}
