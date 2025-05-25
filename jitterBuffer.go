package main

import (
	"encoding/binary"
	"github.com/gordonklaus/portaudio"
	"github.com/hraban/opus"
	"log"
	"net"
	"sync"
	"time"
)

type jitterBuffer struct {
	mu            sync.RWMutex
	frames        map[uint16]*Frame
	expectedSeq   uint16
	lastPacket    time.Time
	resetRequired bool
	initialized   bool
}

func NewJitterBuffer() *jitterBuffer {
	return &jitterBuffer{
		frames: make(map[uint16]*Frame),
	}
}

func (jb *jitterBuffer) AddFrame(seq uint16, data []int16) {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	if !jb.initialized {
		jb.expectedSeq = seq
		jb.initialized = true
		log.Printf("initializing jitterbuffer with seq %d", seq)
	}

	if seq < jb.expectedSeq && jb.expectedSeq-seq > resetThreshold {
		log.Printf("sequence overflow detected (expected %d, got seq %d), resetting", jb.expectedSeq, seq)
		jb.reset()
		jb.expectedSeq = seq
		return
	}

	jb.frames[seq] = &Frame{Data: data}
	jb.lastPacket = time.Now()
}

func (jb *jitterBuffer) GetFrame(seq uint16) (*Frame, bool) {
	jb.mu.RLock()
	defer jb.mu.RUnlock()

	frame, ok := jb.frames[seq]
	return frame, ok
}

func (jb *jitterBuffer) Cleanup(currentSeq uint16) {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	for seq := range jb.frames {
		if seq < currentSeq-uint16(jitterSize/2) {
			delete(jb.frames, seq)
		}
	}
}

func (jb *jitterBuffer) CheckReset() bool {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	if time.Since(jb.lastPacket) > 2*time.Second || jb.resetRequired {
		jb.reset()
		return true
	}
	return false
}

func (jb *jitterBuffer) reset() {
	jb.frames = make(map[uint16]*Frame)
	jb.expectedSeq = 0
	jb.resetRequired = false
	jb.initialized = false
	log.Println("jitterbuffer reset")
}

func (jb *jitterBuffer) findNextAvailableSeq(currentSeq uint16) uint16 {
	jb.mu.RLock()
	defer jb.mu.RUnlock()

	for seq := currentSeq + 1; seq < currentSeq+50; seq++ {
		if _, ok := jb.frames[seq]; ok {
			return seq
		}
	}
	return 0
}

func jitterBufferWorker(conn net.PacketConn, dec *opus.Decoder, playStream *portaudio.Stream, outBuff []int16) {
	jb := NewJitterBuffer()
	silence := make([]int16, len(outBuff))
	var consecutiveMisses int

	ticker := time.NewTicker(frameDurationMs * time.Millisecond)
	defer ticker.Stop()

	// listen channel
	go func() {
		buf := make([]byte, 1500)
		for {
			n, _, err := conn.ReadFrom(buf)
			if err != nil {
				log.Printf("read error %s", err.Error())
				continue
			}

			if n < 2 {
				continue
			}

			seq := binary.BigEndian.Uint16(buf[:2])
			pcm := make([]int16, frameSize*channels)
			_, err = dec.Decode(buf[2:n], pcm)
			if err != nil {
				log.Printf("decode error %s", err.Error())
				continue
			}

			jb.AddFrame(seq, pcm)
		}
	}()

	for range ticker.C {
		if jb.CheckReset() {
			copy(outBuff, silence)
			playStream.Write()
			consecutiveMisses = 0
			continue
		}

		currentSeq := jb.expectedSeq
		frame, ok := jb.GetFrame(currentSeq)

		if ok {
			copy(outBuff, frame.Data)
			jb.Cleanup(currentSeq)
			jb.expectedSeq++
			consecutiveMisses = 0
		} else {
			copy(outBuff, silence)
			if jb.initialized {
				log.Printf("missing frame %d", currentSeq)
				consecutiveMisses++

				if consecutiveMisses > maxFrameMisses {
					if nextSeq := jb.findNextAvailableSeq(currentSeq); nextSeq != 0 {
						log.Printf("jumping from %d to %d", currentSeq, nextSeq)
						jb.expectedSeq = nextSeq
						consecutiveMisses = 0
					}
				}
			}
		}

		if err := playStream.Write(); err != nil {
			log.Printf("play error %s", err.Error())
		}
	}
}
