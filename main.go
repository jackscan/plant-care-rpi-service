package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"

	auth "github.com/abbot/go-http-auth"
	"gobot.io/x/gobot/platforms/raspi"
)

const backlogMinutes = 8 * 60
const backlogDays = 8

type station struct {
	Data    measurementData `json:"data"`
	MinData measurementData `json:"mindata"`
	Config  plantConfig     `json:"config"`

	mutex         sync.RWMutex
	whitelistNets []net.IPNet
	wuc           *Wuc
	cam           *PiCam
	serverConfig  `json:"-"`

	pushCh chan<- bool
}

type measurementData struct {
	Weight   []int `json:"weight"`
	Watering []int `json:"water"`
	Time     int   `json:"time"`
}

type plantConfig struct {
	WaterHour  int `json:"waterhour"`
	WaterStart int `json:"start"`
	MaxWater   int `json:"max"`
	LowLevel   int `json:"low"`
	DstLevel   int `json:"dst"`
	LevelRange int `json:"range"`
	UpdateHour int `json:"updatehour"`
}

type loginConfig struct {
	User string
	Pass string
}

type httpsConfig struct {
	Addr string
	Cert string
	Key  string
}

type filesConfig struct {
	Config     string
	Data       string
	Pictures   string
	PushScript string
}

type serverConfig struct {
	HTTPS httpsConfig
	Login loginConfig
	Files filesConfig
}

func main() {
	log.Print("start")

	var sconfFile string
	flag.StringVar(&sconfFile, "c", "server.conf", "server config file")
	flag.Parse()

	r := raspi.NewAdaptor()
	w, err := NewWuc(r)
	if err != nil {
		log.Fatalf("failed to create connection to microcontroller: %v", err)
	}

	pushCh := make(chan bool, 1)

	s := station{
		serverConfig: serverConfig{
			Login: loginConfig{
				User: "user",
				Pass: "",
			},
			HTTPS: httpsConfig{
				Addr: ":443",
				Cert: "localhost.crt",
				Key:  "localhost.key",
			},
			Files: filesConfig{
				Config:     "/var/opt/plantcare/plant.conf",
				Data:       "/var/opt/plantcare/data.json",
				Pictures:   "/var/opt/plantcare/pics",
				PushScript: "/opt/bin/plantcare-push-pics.sh",
			},
		},
		Config: plantConfig{
			WaterHour:  20,
			WaterStart: 2000,
			MaxWater:   20000,
			LowLevel:   1400,
			DstLevel:   1500,
			LevelRange: 100,
			UpdateHour: 9,
		},
		wuc: w,
		cam: CreatePiCam(),
		Data: measurementData{
			Time:     time.Now().Hour(),
			Weight:   make([]int, 0),
			Watering: make([]int, 0),
		},
		pushCh: pushCh,
	}

	s.parseServerConfigFile(sconfFile)
	s.parsePlantConfigFile()
	s.readData()

	authenticator := auth.NewBasicAuthenticator("plant", s.secret())

	// TODO: create own server instance and do graceful shutdown on signal
	// server := &http.Server{
	// 	Addr: s.serverConfig.HTTPS.Addr,
	// }

	http.Handle("/", http.FileServer(http.Dir("web")))
	http.HandleFunc("/water", auth.JustCheck(authenticator, wateringHandler(&s)))
	http.HandleFunc("/calc", calcWateringHandler(&s))
	http.HandleFunc("/weight", weightHandler(&s))
	http.HandleFunc("/limit", waterLimitHandler(&s))
	http.HandleFunc("/data", dataHandler(&s))
	http.HandleFunc("/config", auth.JustCheck(authenticator, configHandler(&s)))
	http.HandleFunc("/echo", echoHandler(&s))
	http.HandleFunc("/pic", auth.JustCheck(authenticator, pictureHandler(&s)))
	http.HandleFunc("/rotate", auth.JustCheck(authenticator, rotationHandler(&s)))

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go s.run()

	go func() {
		log.Fatal(http.ListenAndServeTLS(
			s.serverConfig.HTTPS.Addr,
			s.serverConfig.HTTPS.Cert,
			s.serverConfig.HTTPS.Key,
			nil))
	}()

	go pushPictures(s.serverConfig.Files.PushScript, s.serverConfig.Files.Pictures, pushCh)

	<-sigs
	log.Print("shutting down")

	s.saveData()
	s.mutex.Lock()
}

func pushPictures(script, folder string, ch <-chan bool) {
	for <-ch {
		log.Println("uploading pictures")
		out, err := exec.Command(script, folder).Output()
		if len(out) > 0 {
			log.Printf("%s\n", out)
		}
		switch e := err.(type) {
		case nil:
		case *exec.ExitError:
			log.Println("failed to push pictures:", string(e.Stderr))
		default:
			log.Printf("failed to execute %s: %v", script, err)
		}
	}
}

func (s *station) parsePlantConfigFile() {
	pc := s.serverConfig.Files.Config
	b, err := ioutil.ReadFile(pc)
	if err != nil && os.IsNotExist(err) {
		log.Printf("plant config %s not found, using default", pc)
		return
	} else if err != nil {
		log.Fatalf("failed to read %s: %v", pc, err)
	}

	err = json.Unmarshal(b, &s.Config)
	if err != nil {
		log.Fatalf("failed to parse plant config: %v", err)
	}
}

func (s *station) parseServerConfigFile(serverConf string) {
	b, err := ioutil.ReadFile(serverConf)
	if err != nil {
		log.Fatalf("failed to read %s: %v", serverConf, err)
	}

	err = toml.Unmarshal(b, &s.serverConfig)
	if err != nil {
		log.Fatalf("failed to parse server config: %v", err)
	}
}

func (s *station) readData() {
	b, err := ioutil.ReadFile(s.serverConfig.Files.Data)
	if err != nil && os.IsNotExist(err) {
		log.Printf("no old measurement data found at %s",
			s.serverConfig.Files.Data)
		return
	} else if err != nil {
		log.Fatalf("failed to read measurement data to %s: %v",
			s.serverConfig.Files.Data, err)
	}

	err = json.Unmarshal(b, &s.Data)
	if err != nil {
		log.Fatalf("failed to marshal measurement data: %v", err)
	}
}

func (s *station) saveData() {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	b, err := json.Marshal(s.Data)
	if err != nil {
		log.Fatalf("failed to marshal measurement data: %v", err)
	}

	err = ioutil.WriteFile(s.serverConfig.Files.Data, b, 0600)
	if err != nil {
		log.Fatalf("failed to save measurement data to %s: %v",
			s.serverConfig.Files.Data, err)
	}
}

func (s *station) run() {
	n := time.Now().Add(60 * time.Minute)
	timer := time.NewTimer(time.Until(time.Date(n.Year(), n.Month(), n.Day(), n.Hour(), 0, 0, 0, n.Location())))

	nm := time.Now().Add(60 * time.Second)
	mintimer := time.NewTimer(time.Until(time.Date(nm.Year(), nm.Month(), nm.Day(), nm.Hour(), nm.Minute(), 0, 0, nm.Location())))

	tch := timer.C
	mtch := mintimer.C

	for {
		select {
		case <-tch:
			// get current hour
			h := time.Now().Add(30 * time.Minute).Hour()
			// next hour
			n := time.Now().Add(90 * time.Minute)
			log.Printf("update %v", h)
			s.update(h)
			// reset timer to next hour
			timer.Reset(time.Until(time.Date(n.Year(), n.Month(), n.Day(), n.Hour(), 0, 0, 0, n.Location())))

		case <-mtch:
			// get current hour
			m := time.Now().Add(30 * time.Second).Minute()
			// next hour
			n := time.Now().Add(90 * time.Second)
			log.Printf("minute %v", m)
			s.updateMinute(m)
			mintimer.Reset(time.Until(time.Date(n.Year(), n.Month(), n.Day(), n.Hour(), n.Minute(), 0, 0, n.Location())))
		}
	}
}

func pushSlice(s []int, v int, maxLen int) []int {
	n := len(s) + 1
	if n > maxLen {
		copy(s, s[n-maxLen:])
		s = s[:maxLen-1]
	}
	return append(s, v)
}

func (s *station) calculateDryoutAndWateringTime() (dryout, wateringTimeScale, wateringTimeOffset int) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// cumulative dryout rate
	dryoutc := 0
	// number of dryout hours accumulated in dryoutc
	dryoutn := 0
	prevw := 0
	prevm := 0
	numw := len(s.Data.Watering)
	numm := len(s.Data.Weight)

	// number of waterings
	wn := float32(0)
	// weight gain sum
	wgsum := float32(0)
	// squared sum of weight gain
	wgsum2 := float32(0)
	// watering time sum
	wtsum := float32(0)
	// dot product of weight gains and watering times
	wgwtdot := float32(0)

	for i, w := range s.Data.Watering {
		if numw-i < numm {
			m := s.Data.Weight[numm-numw+i]
			if prevm > 0 {
				if prevw > 0 {
					fw := float32(prevw)
					wg := float32(m - prevm)
					//log.Printf("w: %v -> %v\n", fw, wg)
					wgsum += wg
					wtsum += fw
					wgsum2 += wg * wg
					wgwtdot += fw * wg
					wn++
				} else {
					// log.Printf("dry: %v\n", prevm-m)
					dryoutc += prevm - m
					dryoutn++
				}
			}
			prevm = m
		}
		prevw = w
	}

	if dryoutn > 0 {
		dryout = dryoutc * 24 / dryoutn
	} else {
		log.Println("no dryout meassured")
		dryout = 0
	}

	if wn > 0 && wgsum*wgsum < wgsum2*wn {
		wts := (wgwtdot - wtsum*wgsum/wn) / (wgsum2 - wgsum*wgsum/wn)
		wateringTimeOffset = int(wtsum/wn - wts*wgsum/wn)
		wateringTimeScale = int(wts)
	} else {
		log.Println("cannot calculate watering times")
		log.Printf("wn: %v\n", wn)
		log.Printf("wgsum: %v\n", wgsum)
		log.Printf("wgsum2: %v\n", wgsum2)
		log.Printf("wtsum: %v\n", wtsum)
		log.Printf("wgwtdot: %v\n", wgwtdot)

		wateringTimeOffset = 0
		wateringTimeScale = 0
	}

	return
}

func (s *station) calculateWatering(hour int, weight int) int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// lastw := (s.Config.WaterStart + s.Config.MaxWater) / 2
	durw := 0

	if len(s.Data.Watering) > 0 {
		for i := len(s.Data.Watering) - 1; i >= 0; i-- {
			if s.Data.Watering[i] > 0 {
				// lastw = s.Data.Watering[i]
				break
			}
			durw = len(s.Data.Watering) - i
		}
	}

	log.Printf("last watered %v hours ago", durw+1)

	sum := weight
	for i := len(s.Data.Weight) - durw; i < len(s.Data.Weight); i++ {
		sum += s.Data.Weight[i]
	}

	avg := sum / (durw + 1)

	log.Printf("average weight since last watering: %v", avg)

	// dl := float32(s.Config.DstLevel - avg)
	// rl := float32(s.Config.LevelRange)
	// rw := float32(s.Config.MaxWater - s.Config.WaterStart)
	// dw := dl / rl * rw

	// log.Printf("adjusting watering time by %v", dw)

	// wt := lastw + int(dw+0.5)

	dryout, wts, wto := s.calculateDryoutAndWateringTime()
	dw := s.Config.DstLevel - weight + dryout/2
	wt := wts*dw + wto

	log.Printf("dryout: %v, wt scale: %v, wt offset: %v, delta weight: %v", dryout, wts, wto, dw)
	log.Printf("watering time: %v", wt)

	return clamp(wt, s.Config.WaterStart, s.Config.MaxWater) - s.Config.WaterStart
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	} else if v > max {
		return max
	}
	return v
}

func hourMedian(mindata []int) int {
	i0 := 0
	n := len(mindata)

	if n == 0 {
		panic(fmt.Errorf("empty slice"))
	}

	if n > 60 {
		i0 = n - 60
	}
	d := make([]int, n-i0)
	copy(d, mindata[i0:])
	sort.Ints(d)
	return d[len(d)/2]
}

func (s *station) updateWeightAndWatering(hour int) {
	var err error
	var w int

	if len(s.MinData.Weight) == 0 {
		w, err = s.wuc.ReadWeight()
		if err != nil {
			log.Printf("failed to read weight: %v", err)

			// fallback to last read weight
			n := len(s.Data.Weight)
			if n > 0 {
				w = s.Data.Weight[n-1]
			}
		}
	} else {
		w = hourMedian(s.MinData.Weight)
	}

	// calculate watering time
	wt := 0
	if hour == s.Config.WaterHour && w <= s.Config.LowLevel {
		wt = s.calculateWatering(hour, w)
	}
	if wt > 0 {
		wt = s.wuc.DoWatering(s.Config.WaterStart, wt)
	}

	// update values
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.Data.Time = hour
	const maxHours = backlogDays * 24
	s.Data.Weight = pushSlice(s.Data.Weight, w, maxHours)
	s.Data.Watering = pushSlice(s.Data.Watering, wt, maxHours)
}

func (s *station) update(hour int) {
	s.updateWeightAndWatering(hour)

	now := time.Now()
	utc := now.UTC()

	if utc.Hour() == s.Config.UpdateHour {
		// calculate angle for picture
		day := utc.Unix() / (24 * 60 * 60)
		angle := uint64(day)

		err := s.wuc.Rotate(angle)
		if err != nil {
			log.Println("failed to rotate plant: ", err)
			return
		}

		evs := []int{-10, 0, 10}
		for i, ev := range evs {
			file, err := s.cam.TakePicture(s.serverConfig.Files.Pictures, ev, 0)
			if err != nil {
				log.Println("failed to take picture:", err)
				if file != "" {
					os.Remove(file)
				}
				continue
			}

			log.Println("image written", file)

			dst := fmt.Sprintf("%s/image-%s-%d.jpg",
				s.serverConfig.Files.Pictures, now.Format("2006-01-02"), i)

			err = os.Rename(file, dst)
			if err != nil {
				log.Printf("failed to move %s to %s: %v", file, dst, err)
				continue
			}

			log.Printf("image moved to %s", dst)
		}

		s.pushCh <- true

		// os.Chdir(s.serverConfig.Files.Pictures)
		// exec.Command("drive", "push", "-files", "-no-prompt", "-no-clobber", "plant")

		angle = uint64(day*10 + 180*(day&1))
		err = s.wuc.Rotate(angle)
		if err != nil {
			log.Println("failed to rotate plant: ", err)
		}
	}
}

func (s *station) updateMinute(min int) {
	w, err := s.wuc.ReadWeight()
	if err != nil {
		log.Printf("failed to read weight: %v", err)
		// fallback to last read weight
		n := len(s.MinData.Weight)
		if n > 0 {
			w = s.MinData.Weight[n-1]
		}
	}

	// update values
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// minutes since last measuring
	numMins := (min - s.MinData.Time + 60) % 60
	if len(s.MinData.Weight) == 0 {
		// no data yet, its first measuring, set to 1
		numMins = 1
	}

	s.MinData.Time = min
	if numMins != 1 {
		log.Printf("missed %v minutes", numMins-1)
	}

	for i := 0; i < numMins; i++ {
		s.MinData.Weight = pushSlice(s.MinData.Weight, w, backlogMinutes)
	}
}

func dataHandler(s *station) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		s.mutex.RLock()
		defer s.mutex.RUnlock()

		js, err := json.Marshal(s)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	}
}

func checkAuth(user, pass string) bool {
	return user == "user" && pass == "pass"
}

func configHandler(s *station) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			s.saveConfig(w, r.Body)
		case http.MethodGet:
			s.sendConfig(w)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func (s *station) saveConfig(w http.ResponseWriter, r io.Reader) {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	c := s.Config
	err = json.Unmarshal(b, &c)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, err)
		return
	}

	b, err = json.Marshal(c)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
		return
	}

	err = ioutil.WriteFile(s.serverConfig.Files.Config, b, 0600)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
	}

	s.Config = c
	fmt.Fprint(w, "config saved")
}

func (s *station) sendConfig(w http.ResponseWriter) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	js, err := json.Marshal(s.Config)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}

func pictureHandler(s *station) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var err error
		ev := 0
		evargs, ok := r.URL.Query()["ev"]
		if ok && len(evargs) > 0 {
			ev, err = strconv.Atoi(evargs[0])
			if err != nil {
				fmt.Fprintf(w, "invalid argument: %v", err)
				return
			}
		}

		shrink := 4
		sargs, ok := r.URL.Query()["s"]
		if ok && len(sargs) > 0 {
			shrink, err = strconv.Atoi(sargs[0])
			if err != nil {
				fmt.Fprintf(w, "invalid argument: %v", err)
				return
			}
		}

		filename, err := s.cam.TakePicture("", ev, uint(shrink))
		if err != nil {
			fmt.Fprint(w, "failed to take picture: ", err)
			return
		}
		defer os.Remove(filename)

		img, err := os.Open(filename)
		if err != nil {
			fmt.Fprint(w, "failed to read image file: ", err)
			return
		}
		defer img.Close()

		w.Header().Set("Content-Type", "image/jpeg")
		io.Copy(w, img)
	}
}

func rotationHandler(s *station) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var err error
		args, ok := r.URL.Query()["a"]

		if !ok || len(args) < 1 {
			fmt.Fprintln(w, "missing argument 'a'")
			return
		}

		a, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Fprintf(w, "invalid argument: %v", err)
			return
		}

		if a < 0 {
			fmt.Fprintln(w, "negative angles not allowed")
			return
		}

		err = s.wuc.Rotate(uint64(a))
		if err != nil {
			fmt.Fprintln(w, "failed to rotate: ", err)
			return
		}

		fmt.Fprintln(w, "rotation finished")
	}
}

func wateringHandler(s *station) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		tq, ok := r.URL.Query()["t"]

		if !ok || len(tq) < 1 {
			t, err := s.wuc.ReadLastWatering()
			if err != nil {
				log.Println("failed to read last watering time: ", err)
			}
			fmt.Fprintf(w, "%v", t)
			return
		}
		t, err := strconv.Atoi(tq[0])
		if err != nil {
			fmt.Fprintf(w, "invalid argument: %v", err)
			return
		}

		st := s.Config.WaterStart
		if len(tq) > 1 {
			st = t
			t, err = strconv.Atoi(tq[1])
			if err != nil {
				fmt.Fprintf(w, "invalid argument: %v", err)
				return
			}
		}

		t = s.wuc.DoWatering(st, t)
		fmt.Fprintf(w, "%v", t)
	}
}

func weightHandler(s *station) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		we, err := s.wuc.ReadWeight()
		if err != nil {
			log.Println("failed to read weight: ", err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, err)
			return
		}
		fmt.Fprintf(w, "%v", we)
	}
}

func waterLimitHandler(s *station) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		m, err := s.wuc.ReadWateringLimit()
		if err != nil {
			log.Println("failed to read watering limit: ", err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, err)
			return
		}
		fmt.Fprintf(w, "%v", m)
	}
}

func calcWateringHandler(s *station) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var err error
		we, err := s.wuc.ReadWeight()
		if err != nil {
			log.Println("failed to read weight: ", err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, err)
			return
		}

		s.mutex.RLock()
		defer s.mutex.RUnlock()

		dryout, wts, wto := s.calculateDryoutAndWateringTime()

		fmt.Fprintf(w, "%v %v %v -> %v", dryout, wts, wto, s.calculateWatering(time.Now().Hour()+1, we))
	}
}

func echoHandler(s *station) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		d := r.URL.Query()["d"]

		var buf []byte

		for _, s := range d {
			i, err := strconv.Atoi(s)
			if err != nil {
				fmt.Fprintf(w, "invalid argument %v: %v", s, err)
				return
			}
			buf = append(buf, byte(i))
		}

		fmt.Fprintf(w, "sending: %v\n", buf)

		buf, err := s.wuc.Echo(buf)
		if err != nil {
			fmt.Fprintf(w, "echo failed: %v", err)
			return
		}

		fmt.Fprintf(w, "%v", buf)
	}
}

func (s *station) secret() func(user, realm string) string {
	return func(user, realm string) string {
		if user == s.serverConfig.Login.User {
			return s.serverConfig.Login.Pass
		}
		return ""
	}
}
