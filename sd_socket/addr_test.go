package sd_socket

import (
	"encoding/binary"
	"net"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestUDPAddrToSockaddr(t *testing.T) {
	tests := []struct {
		name string
		addr *net.UDPAddr
		want unix.Sockaddr
	}{
		{
			name: "IPv4",
			addr: &net.UDPAddr{IP: net.ParseIP("192.0.2.10"), Port: 8080},
			want: &unix.SockaddrInet4{Addr: [4]byte{192, 0, 2, 10}, Port: 8080},
		},
		{
			name: "IPv6",
			addr: &net.UDPAddr{IP: net.ParseIP("2001:db8::10"), Port: 8443, Zone: "7"},
			want: &unix.SockaddrInet6{
				Addr:   [16]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x10},
				Port:   8443,
				ZoneId: 7,
			},
		},
		{
			name: "unspecified IP",
			addr: &net.UDPAddr{Port: 53, Zone: "9"},
			want: &unix.SockaddrInet6{Port: 53, ZoneId: 9},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, udpAddrToSockaddr(tt.addr))
		})
	}
	require.Nil(t, udpAddrToSockaddr(&net.UDPAddr{IP: net.IP{1, 2, 3}, Port: 80}))
}

func TestIP6ZoneConversions(t *testing.T) {
	require.Zero(t, ip6ZoneToInt(""))
	require.Equal(t, 42, ip6ZoneToInt("42"))
	require.Zero(t, ip6ZoneToInt("4294967296"))

	require.Empty(t, ip6ZoneToString(0))
	const missingInterfaceIndex = 1 << 30
	require.Equal(t, "1073741824", ip6ZoneToString(missingInterfaceIndex))
}

func TestSockaddrToUDPAddr(t *testing.T) {
	t.Run("IPv4", func(t *testing.T) {
		addr := SockaddrToUDPAddr(&unix.SockaddrInet4{
			Addr: [4]byte{198, 51, 100, 4},
			Port: 5353,
		})
		require.True(t, addr.IP.Equal(net.ParseIP("198.51.100.4")))
		require.Equal(t, 5353, addr.Port)
	})

	t.Run("IPv6", func(t *testing.T) {
		addr := SockaddrToUDPAddr(&unix.SockaddrInet6{
			Addr:   [16]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4},
			Port:   443,
			ZoneId: 1 << 30,
		})
		require.True(t, addr.IP.Equal(net.ParseIP("2001:db8::4")))
		require.Equal(t, 443, addr.Port)
		require.Equal(t, "1073741824", addr.Zone)
	})

	require.Nil(t, SockaddrToUDPAddr(&unix.SockaddrUnix{Name: "/tmp/stevedore.sock"}))
}

func TestResolveUDPSockaddr(t *testing.T) {
	t.Run("IPv4", func(t *testing.T) {
		resolved := ResolveUDPSockaddr("127.0.0.1:9000")
		require.Equal(t, &unix.SockaddrInet4{
			Addr: [4]byte{127, 0, 0, 1},
			Port: 9000,
		}, resolved)
	})

	t.Run("IPv6", func(t *testing.T) {
		resolved := ResolveUDPSockaddr("[2001:db8::10]:9000")
		require.Equal(t, &unix.SockaddrInet6{
			Addr: [16]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x10},
			Port: 9000,
		}, resolved)
	})

	require.Nil(t, ResolveUDPSockaddr("not a UDP address"))
}

func TestNameBufferToSockaddr(t *testing.T) {
	ipv4Buffer := make([]byte, sizeofSockaddrInet4)
	binary.BigEndian.PutUint16(ipv4Buffer[2:4], 8080)
	copy(ipv4Buffer[4:8], net.IPv4(192, 0, 2, 20).To4())
	require.Equal(t, &unix.SockaddrInet4{
		Addr: [4]byte{192, 0, 2, 20},
		Port: 8080,
	}, nameBufferToSockaddr(ipv4Buffer, sizeofSockaddrInet4))

	ipv6Buffer := make([]byte, sizeofSockaddrInet6)
	binary.BigEndian.PutUint16(ipv6Buffer[2:4], 8443)
	copy(ipv6Buffer[8:24], net.ParseIP("2001:db8::20").To16())
	binary.BigEndian.PutUint32(ipv6Buffer[24:28], 12)
	require.Equal(t, &unix.SockaddrInet6{
		Addr:   [16]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x20},
		Port:   8443,
		ZoneId: 12,
	}, nameBufferToSockaddr(ipv6Buffer, sizeofSockaddrInet6))

	require.Nil(t, nameBufferToSockaddr(make([]byte, sizeofSockaddrInet6), 0))
}

func TestNameBufferToSockaddrRejectsShortBuffer(t *testing.T) {
	require.Nil(t, nameBufferToSockaddr(make([]byte, sizeofSockaddrInet4-1), sizeofSockaddrInet4))
	require.Nil(t, nameBufferToSockaddr(make([]byte, sizeofSockaddrInet6-1), sizeofSockaddrInet6))
}

func TestNameBufferToUDPAddr(t *testing.T) {
	t.Run("IPv4", func(t *testing.T) {
		buf, nameLen := udpAddrToNameBuffer(&net.UDPAddr{
			IP:   net.ParseIP("203.0.113.8"),
			Port: 5353,
		})
		addr := nameBufferToUDPAddr(buf, nameLen)
		require.True(t, addr.IP.Equal(net.ParseIP("203.0.113.8")))
		require.Equal(t, 5353, addr.Port)
	})

	t.Run("IPv6", func(t *testing.T) {
		const missingInterfaceZone = "1073741824"
		buf, nameLen := udpAddrToNameBuffer(&net.UDPAddr{
			IP:   net.ParseIP("2001:db8::8"),
			Port: 8443,
			Zone: missingInterfaceZone,
		})
		addr := nameBufferToUDPAddr(buf, nameLen)
		require.True(t, addr.IP.Equal(net.ParseIP("2001:db8::8")))
		require.Equal(t, 8443, addr.Port)
		require.Equal(t, missingInterfaceZone, addr.Zone)
	})

	require.Nil(t, nameBufferToUDPAddr(make([]byte, sizeofSockaddrInet6), 0))
}

func TestNameBufferToUDPAddrRejectsShortBuffer(t *testing.T) {
	require.Nil(t, nameBufferToUDPAddr(make([]byte, sizeofSockaddrInet4-1), sizeofSockaddrInet4))
	require.Nil(t, nameBufferToUDPAddr(make([]byte, sizeofSockaddrInet6-1), sizeofSockaddrInet6))
}

func TestSockaddrToNameBuffer(t *testing.T) {
	tests := []struct {
		name     string
		addr     unix.Sockaddr
		wantLen  uint32
		wantIP   []byte
		wantPort uint16
		wantZone uint32
	}{
		{
			name:     "IPv4",
			addr:     &unix.SockaddrInet4{Port: 8080, Addr: [4]byte{192, 0, 2, 1}},
			wantLen:  sizeofSockaddrInet4,
			wantIP:   []byte{192, 0, 2, 1},
			wantPort: 8080,
		},
		{
			name:     "IPv6",
			addr:     &unix.SockaddrInet6{Port: 443, Addr: [16]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}, ZoneId: 7},
			wantLen:  sizeofSockaddrInet6,
			wantIP:   []byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
			wantPort: 443,
			wantZone: 7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf, nameLen := sockaddrToNameBuffer(tt.addr)
			if nameLen != tt.wantLen {
				t.Fatalf("name length = %d, want %d", nameLen, tt.wantLen)
			}
			if len(buf) != int(tt.wantLen) {
				t.Fatalf("buffer length = %d, want %d", len(buf), tt.wantLen)
			}
			if port := binary.BigEndian.Uint16(buf[2:4]); port != tt.wantPort {
				t.Errorf("port = %d, want %d", port, tt.wantPort)
			}

			ipOffset := 4
			if tt.wantLen == sizeofSockaddrInet6 {
				ipOffset = 8
			}
			if got := net.IP(buf[ipOffset : ipOffset+len(tt.wantIP)]); !got.Equal(net.IP(tt.wantIP)) {
				t.Errorf("IP = %v, want %v", got, net.IP(tt.wantIP))
			}
			if tt.wantLen == sizeofSockaddrInet6 {
				if zone := binary.BigEndian.Uint32(buf[24:28]); zone != tt.wantZone {
					t.Errorf("zone = %d, want %d", zone, tt.wantZone)
				}
			}
		})
	}
}

func TestSockaddrToNameBufferRejectsInvalidAddress(t *testing.T) {
	tests := []unix.Sockaddr{
		nil,
		&unix.SockaddrUnix{Name: "/tmp/test.sock"},
		&unix.SockaddrInet4{Port: -1},
		&unix.SockaddrInet6{Port: 1 << 16},
	}

	for _, addr := range tests {
		buf, nameLen := sockaddrToNameBuffer(addr)
		if buf != nil || nameLen != 0 {
			t.Errorf("sockaddrToNameBuffer(%T) = (%v, %d), want (nil, 0)", addr, buf, nameLen)
		}
	}
}

func TestUDPAddrToNameBuffer(t *testing.T) {
	tests := []struct {
		name     string
		addr     *net.UDPAddr
		wantLen  uint32
		wantIP   net.IP
		wantPort uint16
		wantZone uint32
	}{
		{
			name:     "IPv4",
			addr:     &net.UDPAddr{IP: net.ParseIP("198.51.100.2"), Port: 5353},
			wantLen:  sizeofSockaddrInet4,
			wantIP:   net.ParseIP("198.51.100.2"),
			wantPort: 5353,
		},
		{
			name:     "IPv6",
			addr:     &net.UDPAddr{IP: net.ParseIP("2001:db8::2"), Port: 8443, Zone: "9"},
			wantLen:  sizeofSockaddrInet6,
			wantIP:   net.ParseIP("2001:db8::2"),
			wantPort: 8443,
			wantZone: 9,
		},
		{
			name:     "unspecified IP",
			addr:     &net.UDPAddr{Port: 53},
			wantLen:  sizeofSockaddrInet6,
			wantIP:   net.IPv6zero,
			wantPort: 53,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf, nameLen := udpAddrToNameBuffer(tt.addr)
			if nameLen != tt.wantLen {
				t.Fatalf("name length = %d, want %d", nameLen, tt.wantLen)
			}
			if port := binary.BigEndian.Uint16(buf[2:4]); port != tt.wantPort {
				t.Errorf("port = %d, want %d", port, tt.wantPort)
			}

			ipOffset, ipLength := 4, net.IPv4len
			if tt.wantLen == sizeofSockaddrInet6 {
				ipOffset, ipLength = 8, net.IPv6len
			}
			if got := net.IP(buf[ipOffset : ipOffset+ipLength]); !got.Equal(tt.wantIP) {
				t.Errorf("IP = %v, want %v", got, tt.wantIP)
			}
			if tt.wantLen == sizeofSockaddrInet6 {
				if zone := binary.BigEndian.Uint32(buf[24:28]); zone != tt.wantZone {
					t.Errorf("zone = %d, want %d", zone, tt.wantZone)
				}
			}
		})
	}
}

func TestUDPAddrToNameBufferRejectsInvalidAddress(t *testing.T) {
	tests := []*net.UDPAddr{
		nil,
		{IP: net.IP{1, 2, 3}, Port: 80},
		{IP: net.IPv4(127, 0, 0, 1), Port: -1},
		{IP: net.IPv6loopback, Port: 1 << 16},
	}

	for _, addr := range tests {
		buf, nameLen := udpAddrToNameBuffer(addr)
		if buf != nil || nameLen != 0 {
			t.Errorf("udpAddrToNameBuffer(%v) = (%v, %d), want (nil, 0)", addr, buf, nameLen)
		}
	}
}

func TestUdpAddrToSockaddrConvey(t *testing.T) {
	convey.Convey("Test udpAddrToSockaddr", t, func() {
		convey.Convey("converts IPv4", func() {
			got := udpAddrToSockaddr(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 80})
			addr, ok := got.(*unix.SockaddrInet4)
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(addr.Port, convey.ShouldEqual, 80)
			convey.So(addr.Addr, convey.ShouldResemble, [4]byte{127, 0, 0, 1})
		})
	})
}

func TestIp6ZoneToIntConvey(t *testing.T) {
	convey.Convey("Test ip6ZoneToInt", t, func() {
		convey.Convey("resolves a mocked interface", func() {
			patches := gomonkey.ApplyFuncReturn(
				net.InterfaceByName,
				&net.Interface{Index: 7},
				nil,
			)
			defer patches.Reset()
			convey.So(ip6ZoneToInt("mock0"), convey.ShouldEqual, 7)
		})
	})
}

func TestSockaddrToUDPAddrConvey(t *testing.T) {
	convey.Convey("Test SockaddrToUDPAddr", t, func() {
		convey.Convey("converts IPv6", func() {
			got := SockaddrToUDPAddr(&unix.SockaddrInet6{
				Addr: [16]byte{0: 0x20, 1: 0x01, 15: 0x01},
				Port: 443,
			})
			convey.So(got.Port, convey.ShouldEqual, 443)
			convey.So(got.IP, convey.ShouldResemble, net.IP{0x20, 0x01, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})
		})
	})
}

func TestIp6ZoneToStringConvey(t *testing.T) {
	convey.Convey("Test ip6ZoneToString", t, func() {
		convey.Convey("resolves a mocked interface", func() {
			patches := gomonkey.ApplyFuncReturn(
				net.InterfaceByIndex,
				&net.Interface{Name: "mock0"},
				nil,
			)
			defer patches.Reset()
			convey.So(ip6ZoneToString(7), convey.ShouldEqual, "mock0")
		})
	})
}

func TestResolveUDPSockaddrConvey(t *testing.T) {
	convey.Convey("Test ResolveUDPSockaddr", t, func() {
		convey.Convey("resolves a UDP address", func() {
			got := ResolveUDPSockaddr("127.0.0.1:8080")
			addr, ok := got.(*unix.SockaddrInet4)
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(addr.Port, convey.ShouldEqual, 8080)
		})
	})
}

func TestNameBufferToSockaddrConvey(t *testing.T) {
	convey.Convey("Test nameBufferToSockaddr", t, func() {
		convey.Convey("decodes IPv4", func() {
			buf := make([]byte, sizeofSockaddrInet4)
			buf[2], buf[3] = 0x1f, 0x90
			copy(buf[4:8], []byte{127, 0, 0, 1})
			got := nameBufferToSockaddr(buf, sizeofSockaddrInet4).(*unix.SockaddrInet4)
			convey.So(got.Port, convey.ShouldEqual, 8080)
			convey.So(got.Addr, convey.ShouldResemble, [4]byte{127, 0, 0, 1})
		})
	})
}

func TestSockaddrToNameBufferConvey(t *testing.T) {
	convey.Convey("Test sockaddrToNameBuffer", t, func() {
		convey.Convey("encodes IPv4", func() {
			buf, length := sockaddrToNameBuffer(&unix.SockaddrInet4{
				Addr: [4]byte{127, 0, 0, 1},
				Port: 8080,
			})
			convey.So(length, convey.ShouldEqual, sizeofSockaddrInet4)
			convey.So(buf[2:8], convey.ShouldResemble, []byte{0x1f, 0x90, 127, 0, 0, 1})
		})
	})
}

func TestNameBufferToUDPAddrConvey(t *testing.T) {
	convey.Convey("Test nameBufferToUDPAddr", t, func() {
		convey.Convey("decodes a UDP address", func() {
			buf := make([]byte, sizeofSockaddrInet4)
			buf[2], buf[3] = 0, 80
			copy(buf[4:8], []byte{192, 0, 2, 1})
			got := nameBufferToUDPAddr(buf, sizeofSockaddrInet4)
			convey.So(got.Port, convey.ShouldEqual, 80)
			convey.So(got.IP, convey.ShouldResemble, net.IP{192, 0, 2, 1})
		})
	})
}

func TestUdpAddrToNameBufferConvey(t *testing.T) {
	convey.Convey("Test udpAddrToNameBuffer", t, func() {
		convey.Convey("round trips IPv4", func() {
			buf, length := udpAddrToNameBuffer(&net.UDPAddr{
				IP: net.IPv4(127, 0, 0, 1), Port: 53,
			})
			got := nameBufferToUDPAddr(buf, length)
			convey.So(got.Port, convey.ShouldEqual, 53)
			convey.So(got.IP, convey.ShouldResemble, net.IP{127, 0, 0, 1})
		})
	})
}
