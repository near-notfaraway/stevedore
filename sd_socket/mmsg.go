package sd_socket

const (
	minMMsgContainerBatchNum = 32
	minMMsgContainerMTU      = 4096
)

type SockaddrKey struct {
	nameLen uint32
	name    [sizeofSockaddrInet6]byte
}

func normalizeMMsgSize(batchNum, mtu int) (int, int) {
	if batchNum < minMMsgContainerBatchNum {
		batchNum = minMMsgContainerBatchNum
	}
	if mtu < minMMsgContainerMTU {
		mtu = minMMsgContainerMTU
	}
	return batchNum, mtu
}
