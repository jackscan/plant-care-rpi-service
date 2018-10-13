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
func (c *PiCam) TakePicture(folder string, ev int, s uint) (string, error) {

	c.mutex.Lock()
	defer c.mutex.Unlock()

	f, err := ioutil.TempFile(folder, "image-")
	if err != nil {
		return "", err
	}

	filename := f.Name()
	f.Close()

	log.Println("image ", filename, ", ev: ", ev)

	w := 2464
	h := 3280

	if s != 0 && s < uint(w) {
		w = w / int(s)
		h = h / int(s)
	}

	args := []string{
		"-o", filename,
		"-ISO", "100",
		"-ev", strconv.Itoa(ev),
		"-ex", "backlight",
		"-mm", "average",
		"-awb", "off",
		"-awbg", "1.7,1.6",
		"-ag", "1.0",
		"-dg", "1.0",
		"-t", "2000",
		"-w", strconv.Itoa(w),
		"-h", strconv.Itoa(h),
		"-rot", "90",
	}

	cmd := exec.Command(c.exe, args...)
	return filename, cmd.Run()
}
