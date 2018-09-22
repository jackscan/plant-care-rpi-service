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
func (c *PiCam) TakePicture(folder string, shutter int) (string, error) {

	c.mutex.Lock()
	defer c.mutex.Unlock()

	f, err := ioutil.TempFile(folder, "image-")
	if err != nil {
		return "", err
	}

	filename := f.Name()
	f.Close()

	log.Println("image ", filename, ", shutter: ", shutter)

	args := []string{
		"-o", filename,
		"-ISO", "100",
		"-ss", strconv.Itoa(shutter),
		"-ex", "backlight",
		"-mm", "average",
		"-awb", "off",
		"-awbg", "1.7,1.6",
		"-ag", "1.0",
		"-dg", "1.0",
		"-t", "2",
	}

	cmd := exec.Command(c.exe, args...)
	return filename, cmd.Run()
}
