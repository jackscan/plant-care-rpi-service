package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"gobot.io/x/gobot/drivers/i2c"
)

const (
	cmdGetLastWatering = 0x10
	cmdGetWaterLimit   = 0x11
	cmdGetWeight       = 0x12
	cmdRotate          = 0x13
	cmdStop            = 0x14
	cmdGetMotorStatus  = 0x15
	cmdWatering        = 0x1A
	cmdEcho            = 0x29
)

// CPR is the counts per revolution of rotating plate.
const CPR = 15808

// A Wuc provides the interface to the Watering Micro Controller.
type Wuc struct {
	connection i2c.Connection
	mutex      *sync.Mutex
}

// NewWuc creates an instance of a Wuc.
func NewWuc(c i2c.Connector) (*Wuc, error) {
	connection, err := c.GetConnection(0x10, 1)
	if err != nil {
		return nil, err
	}

	return &Wuc{
		connection: connection,
		mutex:      &sync.Mutex{},
	}, nil
}

// ReadWeight triggers read of weight sensor.
func (w *Wuc) ReadWeight() (m int, err error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if err = w.connection.WriteByte(cmdGetWeight); err != nil {
		return
	}

	time.Sleep(700 * time.Millisecond)

	var buf [2]byte
	n, err := w.connection.Read(buf[:])
	if err != nil {
		return
	}

	if n != 2 {
		return 0, fmt.Errorf("invalid length of result: %d", n)
	}

	if buf[1] == 0xFF {
		return 0, fmt.Errorf("failed to measure weight")
	}

	m = (int(buf[1]) << 8) | int(buf[0])

	return
}

func (w *Wuc) waitForStop(timeout int) error {
	for i := 0; i < timeout; i++ {
		// wait a second before checking status
		time.Sleep(time.Second)

		var buf [2]byte
		n, err := w.connection.Read(buf[:])
		if err != nil {
			log.Println("failed to read motor status")
			continue
		}

		if n != 2 {
			log.Printf("invalid length of motor status: %d", n)
			continue
		}

		feed := buf[0]
		skip := buf[1] & 0x3f
		running := (buf[1] & 0x80) != 0
		calibrated := (buf[1] & 0x40) != 0

		log.Printf("motor: feed: %v, skip: %v, calibrated: %v, running %v", feed, skip, calibrated, running)

		if !running {
			return nil
		}
	}

	w.connection.WriteByte(cmdStop)
	return fmt.Errorf("motor did not finish in time")
}

// Rotate sends rotate command and waits for it to finish.
func (w *Wuc) Rotate(angle uint64) error {

	a := uint((angle * CPR / 360) % CPR)

	w.mutex.Lock()
	defer w.mutex.Unlock()

	log.Printf("rotating to %v(%v°)", a, a*360/CPR)

	cmd := []byte{
		cmdRotate,
		byte(a & 0xFF),
		byte((a >> 8) & 0xFF),
	}

	n, err := w.connection.Write(cmd)
	if err != nil {
		return fmt.Errorf("failed to send watering command: %v", err)
	}

	if n < len(cmd) {
		return fmt.Errorf("could not send complete watering command: %v/%v", n, len(cmd))
	}

	// wait at most 20 seconds
	return w.waitForStop(20)
}

// DoWatering sends command for watering.
func (w *Wuc) DoWatering(start, watering int) int {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	s := (start + 125) / 250
	if s < 0 || s > 255 {
		log.Printf("watering start time out of range: %v(%v)", s, start)
		return 0
	}

	u := (watering + 125) / 250
	if u < 0 || u > 255 {
		log.Printf("watering time out of range: %v(%v)", u, watering)
		return 0
	}

	log.Printf("watering %v+%v ms", s*250, u*250)
	cmd := []byte{cmdWatering, byte(s), byte(u)}

	n, err := w.connection.Write(cmd)
	if err != nil {
		log.Printf("failed to send watering command: %v", err)
		return 0
	}

	if n < len(cmd) {
		log.Printf("could not send complete watering command: %v/%v", n, len(cmd))
		return 0
	}

	// wait for:
	//  - for watering to finish
	//  - and some margin
	time.Sleep(time.Duration(start+watering+500) * time.Millisecond)

	// might return 0 when rotation takes longer than desired
	r, err := w.connection.ReadByte()

	if err != nil {
		log.Printf("failed to read watering time: %v", err)
		return 0
	}

	// 0 when motor probably not yet finished
	if r == 0 {
		// check and wait until motor is actually stopped before checking watering result
		err = w.connection.WriteByte(cmdGetMotorStatus)
		if err == nil {
			err = w.waitForStop(20)
		}

		if err != nil {
			log.Println("failure on waiting for motor:", err)
		}

		r, err = w.readLastWatering()
		if err != nil {
			log.Println(err)
		}
	}

	if int(r) != u {
		log.Printf("watered %v ms", int(r)*250)
	}

	return int(r) * 250
}

// ReadLastWatering queries duration of last watering and returns time in ms.
func (w *Wuc) ReadLastWatering() (int, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	t, err := w.readLastWatering()

	if err != nil {
		return 0, err
	}

	return int(t) * 250, nil
}

func (w *Wuc) readLastWatering() (byte, error) {
	if err := w.connection.WriteByte(cmdGetLastWatering); err != nil {
		return 0, err
	}

	t, err := w.connection.ReadByte()
	if err != nil {
		return 0, err
	}

	if t == 0xFF {
		return 0, fmt.Errorf("failed to get last watering time")
	}
	return t, nil
}

// ReadWateringLimit sends command to measure water Limit and returns result.
func (w *Wuc) ReadWateringLimit() (int, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if err := w.connection.WriteByte(cmdGetWaterLimit); err != nil {
		return 0, err
	}

	l, err := w.connection.ReadByte()
	if err != nil {
		return 0, err
	}

	if l == 0xFF {
		return 0, fmt.Errorf("failed to measure water Limit")
	}

	return int(l), nil
}

// Echo sends echo command with data of given buffer and returns result.
func (w *Wuc) Echo(buf []byte) ([]byte, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	b := make([]byte, len(buf)+1)
	b[0] = cmdEcho
	copy(b[1:], buf)

	if _, err := w.connection.Write(b); err != nil {
		return nil, err
	}

	n, err := w.connection.Read(b)
	if err != nil {
		return nil, err
	}

	b = b[:n]

	return b, err
}
