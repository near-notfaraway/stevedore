package sd_socket

const (
	MinMMsgContainerBatchNum = 32
	MinMMsgContainerMTU      = 4096
)

type SockaddrKey struct {
	NameLen uint32
	Name    [SizeofSockaddrInet6]byte
}

func normalizeMMsgSize(batchNum, mtu int) (int, int) {
	if batchNum < MinMMsgContainerBatchNum {
		batchNum = MinMMsgContainerBatchNum
	}
	if mtu < MinMMsgContainerMTU {
		mtu = MinMMsgContainerMTU
	}
	return batchNum, mtu
}
