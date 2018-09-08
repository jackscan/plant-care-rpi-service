package main

import (
	"io/ioutil"
	"log"
	"os/exec"
	"strconv"
	"sync"
)

// PiCam is wrapper for raspistill command.
type PiCam struct {
	exe string
	// args  []string
	mutex *sync.Mutex
}

// CreatePiCam creates a PiCam instance.
func CreatePiCam() *PiCam {
	return &PiCam{
		exe: "/opt/vc/bin/raspistill",
		// args: []string{
		// 	"-o", "/tmp/image.jpg",
		// },
		mutex: &sync.Mutex{},
	}
}

// TakePicture makes a picture with given exposure compensation value.
func (c *PiCam) TakePicture(folder string, ev int) (string, error) {

	c.mutex.Lock()
	defer c.mutex.Unlock()

	f, err := ioutil.TempFile(folder, "image-")
	if err != nil {
		return "", err
	}

	filename := f.Name()
	f.Close()

	log.Println("image ", filename, ", ev: ", ev)

	args := []string{
		"-o", filename,
		"--exposure", "verylong",
		"-t", "1",
		"-ev", strconv.Itoa(ev),
	}

	cmd := exec.Command(c.exe, args...)
	return filename, cmd.Run()
}
