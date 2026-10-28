package launcher

import (
	"strings"
	"sync"
)

type outputTail struct {
	mutex         sync.Mutex
	capacityBytes int
	storedBytes   []byte
}

func newOutputTail(capacityBytes int) *outputTail {
	return &outputTail{capacityBytes: capacityBytes}
}

func (tail *outputTail) Write(outputChunk []byte) (int, error) {
	tail.mutex.Lock()
	defer tail.mutex.Unlock()

	tail.storedBytes = append(tail.storedBytes, outputChunk...)
	if extraBytes := len(tail.storedBytes) - tail.capacityBytes; extraBytes > 0 {
		tail.storedBytes = append([]byte(nil), tail.storedBytes[extraBytes:]...)
	}
	return len(outputChunk), nil
}

func (tail *outputTail) String() string {
	tail.mutex.Lock()
	defer tail.mutex.Unlock()
	return strings.TrimSpace(strings.ToValidUTF8(string(tail.storedBytes), ""))
}
