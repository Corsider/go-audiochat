package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"github.com/gordonklaus/portaudio"
	"github.com/hraban/opus"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
)

const (
	sampleRate      = 48000
	channels        = 1
	frameSize       = sampleRate * frameDurationMs / 1000
	frameDurationMs = 20
	jitterSize      = 300
	resetThreshold  = 100
	maxFrameMisses  = 10
)

type Frame struct {
	Data []int16
}

func main() {
	if len(os.Args) != 5 {
		fmt.Println("Аргументы: -d {{addr}} -l {{addr}}, где -d адрес абонента, -l порт на котором отдаем звук, например :9000")
		return
	}
	targetAddr := os.Args[2]
	listenAddr := os.Args[4]
	err := portaudio.Initialize()
	if err != nil {
		panic(err)
	}
	defer portaudio.Terminate()
	enc, err := opus.NewEncoder(sampleRate, channels, opus.AppVoIP)
	if err != nil {
		log.Fatal(err)
	}
	dec, err := opus.NewDecoder(sampleRate, channels)
	if err != nil {
		log.Fatal(err)
	}

	listenCfg := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				syscall.SetsockoptInt(syscall.Handle(int(fd)), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
			})
		},
	}

	//conn, err := net.ListenPacket("udp", listenAddr)
	conn, err := listenCfg.ListenPacket(context.Background(), "udp", listenAddr)
	if err != nil {
		log.Fatal(err)
	}

	defer conn.Close()

	remoteAddr, err := net.ResolveUDPAddr("udp", targetAddr)
	if err != nil {
		log.Fatal(err)
	}

	inBuf := make([]int16, frameSize*channels)
	outBuf := make([]int16, frameSize*channels)
	inputStream, err := portaudio.OpenDefaultStream(channels, 0, float64(sampleRate), frameSize, inBuf)
	if err != nil {
		log.Fatal(err)
	}
	defer inputStream.Close()

	playStream, err := portaudio.OpenDefaultStream(0, channels, float64(sampleRate), frameSize, outBuf)
	if err != nil {
		log.Fatal(err)
	}

	defer playStream.Close()

	err = inputStream.Start()
	if err != nil {
		log.Fatal(err)
	}

	err = playStream.Start()
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Слушаем ", targetAddr, " отдаем на ", listenAddr)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)

	go func() {
		//write chan
		var seq uint16
		buf := make([]byte, 1500)

		for {
			err := inputStream.Read()
			if err != nil {
				log.Println(err)
				continue
			}
			n, err := enc.Encode(inBuf, buf[2:])
			if err != nil {
				log.Println(err)
				continue
			}

			binary.BigEndian.PutUint16(buf[:2], seq)
			_, err = conn.WriteTo(buf[:n+2], remoteAddr)
			if err != nil {
				log.Println(err)
				//continue
			}
			seq++
		}
	}()

	go jitterBufferWorker(conn, dec, playStream, outBuf)

	<-stop
	log.Println("exiting...")
}
