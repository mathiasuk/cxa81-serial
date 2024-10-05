package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sync"
	"time"

	"go.bug.st/serial"
)

// Command represents a serial command to the CXA amplifier.
type Command struct {
	Group  string
	Number string
	Data   string
}

// Amplifier Commands
var (
	GetPowerState   = Command{Group: "01", Number: "01"}
	SetPowerStandby = Command{Group: "01", Number: "02", Data: "0"}
	SetPowerOn      = Command{Group: "01", Number: "02", Data: "1"}
	GetMuteState    = Command{Group: "01", Number: "03"}
	SetMuteOff      = Command{Group: "01", Number: "04", Data: "0"}
	SetMuteOn       = Command{Group: "01", Number: "04", Data: "1"}
)

// Source Commands
var (
	GetSource           = Command{Group: "03", Number: "01"}
	GetNextSource       = Command{Group: "03", Number: "02"}
	GetPreviousSource   = Command{Group: "03", Number: "03"}
	SetSourceA1         = Command{Group: "03", Number: "04", Data: "00"}
	SetSourceA2         = Command{Group: "03", Number: "04", Data: "01"}
	SetSourceA3         = Command{Group: "03", Number: "04", Data: "02"}
	SetSourceA4         = Command{Group: "03", Number: "04", Data: "03"}
	SetSourceD1         = Command{Group: "03", Number: "04", Data: "04"}
	SetSourceD2         = Command{Group: "03", Number: "04", Data: "05"}
	SetSourceD3         = Command{Group: "03", Number: "04", Data: "06"}
	SetSourceMP3        = Command{Group: "03", Number: "04", Data: "10"}
	SetSourceBluetooth  = Command{Group: "03", Number: "04", Data: "14"}
	SetSourceUSBAudio   = Command{Group: "03", Number: "04", Data: "16"}
	SetSourceA1Balanced = Command{Group: "03", Number: "04", Data: "20"}
)

// Version Commands
var (
	GetProtocolVersion = Command{Group: "13", Number: "01"}
	GetFirmwareVersion = Command{Group: "13", Number: "02"}
)

// Sources
var sources = map[string]string{
	"00": "A1",
	"01": "A2",
	"02": "A3",
	"03": "A4",
	"04": "D1",
	"05": "D2",
	"06": "D3",
	"10": "MP3", // CXA81 only
	"14": "Bluetooth",
	"16": "USB",
	"20": "A1 Balanced",
}

// Reply represents a reply from the CXA amplifier.
type Reply struct {
	Group  string
	Number string
	Data   string
}

var validReply = regexp.MustCompile(`#(\d\d),(\d\d)(?:,([^\r]*))?\r`)

func (r *Reply) String() string {
	desc := fmt.Sprintf("Unknown reply: %s,%s,%s", r.Group, r.Number, r.Data)
	data := r.Data

	switch r.Group {
	case "00":
		switch r.Number {
		case "01":
			desc = "Command group unknown"
		case "02":
			desc = "Command number unknown"
		case "03":
			desc = "Command data error"
		case "04":
			desc = "Command not available"
		}
	case "02":
		switch r.Number {
		case "01":
			desc = "Current power state"
		case "03":
			desc = "Current mute state"
		}
	case "04":
		if r.Number == "01" {
			desc = "Current source"
			data = sources[data]
		}
	case "14":
		switch r.Number {
		case "01":
			desc = "Protocol Version"
		case "02":
			desc = "Get Firmware Version"
		}
	}

	if r.Data != "" {
		return fmt.Sprintf("%s: %s", desc, r.Data)
	}

	return desc
}

// AmplifierState represents the internal state of the amplifier.
type AmplifierState struct {
	Powered bool
	Muted   bool
	Source  string
}

// Amplifier represents the CXA amplifier and its serial connection.
type Amplifier struct {
	port serial.Port

	mu    sync.Mutex
	state AmplifierState
}

// NewAmplifier creates a new Amplifier instance.
func NewAmplifier(portName string) (*Amplifier, error) {
	mode := &serial.Mode{
		BaudRate: 9600,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}

	port, err := serial.Open(portName, mode)
	if err != nil {
		return nil, err
	}

	return &Amplifier{port: port}, nil
}

// SendCommand sends a command to the amplifier.
func (a *Amplifier) SendCommand(cmd Command) error {
	s := fmt.Sprintf("#%s,%s", cmd.Group, cmd.Number)
	if cmd.Data != "" {
		s += fmt.Sprintf(",%s\r", cmd.Data)
	} else {
		s += "\r"
	}

	_, err := a.port.Write([]byte(s))
	if err != nil {
		return err
	}

	return nil
}

// Read returns a Reply from the amp.
func (a *Amplifier) Listen() {
	buf := make([]byte, 1024)

	for {
		n, err := a.port.Read(buf)
		if err != nil {
			log.Panicf("Read(): %v", err)
		}

		response := string(buf[:n])
		matches := validReply.FindAllStringSubmatch(response, -1)
		if matches == nil {
			log.Panicf("invalid reply format: %q", response)
		}

		for _, m := range matches {
			reply := &Reply{
				Group:  m[1],
				Number: m[2],
			}
			// If data is present, capture it
			if len(m) > 3 {
				reply.Data = m[3]
			}
			log.Printf("Received: %v", reply)
			a.UpdateState(reply)
		}
		time.Sleep(1 * time.Second)
	}
}

func (a *Amplifier) UpdateState(r *Reply) {
	a.mu.Lock()
	defer a.mu.Unlock()
	switch r.Group {
	case "02":
		switch r.Number {
		case "01":
			a.state.Powered = r.Data == "1"

			// Powering off resets the muted state.
			if !a.state.Powered {
				a.state.Muted = false
			}
		case "03":
			a.state.Muted = r.Data == "1"
		}
	case "04":
		if r.Number == "01" {
			a.state.Source = sources[r.Data]
		}
	}
}

// ServeHTTP serves the amplifier status.
func (a *Amplifier) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// TODO: Auth.

	a.mu.Lock()
	defer a.mu.Unlock()

	if r.Method == "POST" {
		var req struct {
			Power  string
			Mute   string
			Source string
		}
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := a.handlePower(req.Power); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		if err := a.handleMute(req.Mute); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		if err := a.handleSource(req.Mute); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}

	// GET
	json.NewEncoder(w).Encode(a.state)
}

// handlePower updates the power status from the given string.
func (a *Amplifier) handlePower(s string) error {
	var c Command

	switch s {
	case "on":
		c = SetPowerOn
	case "off":
		c = SetPowerStandby
	case "toggle":
		if a.state.Powered {
			c = SetPowerStandby
		} else {
			c = SetPowerOn
		}
	case "":
		return nil
	default:
		return fmt.Errorf("Unexpected power state %s, expected: on/off/toggle", s)
	}

	return a.SendCommand(c)
}

// handleMute updates the mute status from the given string.
func (a *Amplifier) handleMute(s string) error {
	var c Command

	switch s {
	case "on", "muted":
		c = SetMuteOn
	case "off", "unmuted":
		c = SetMuteOff
	case "":
		return nil
	default:
		return fmt.Errorf("Unexpected mute state %s, expected: on/off/muted/unmuted", s)
	}

	return a.SendCommand(c)
}

// handleSource updates the source from the given string.
func (a *Amplifier) handleSource(s string) error {
	var c Command

	switch s {
	case "A1":
		c = SetSourceA1
	case "A2":
		c = SetSourceA2
	case "A3":
		c = SetSourceA3
	case "A4":
		c = SetSourceA4
	case "D1":
		c = SetSourceD1
	case "D2":
		c = SetSourceD2
	case "D3":
		c = SetSourceD3
	case "MP3":
		c = SetSourceMP3
	case "Bluetooth":
		c = SetSourceBluetooth
	case "USB":
		c = SetSourceUSBAudio
	case "A1 Balanced":
		c = SetSourceA1Balanced
	case "":
		return nil
	default:
		return fmt.Errorf("Unknown source: %s", s)
	}

	return a.SendCommand(c)
}

func (a *Amplifier) HTTPSource(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	var req struct {
		Source string
	}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var c Command
	switch req.Source {
	case "A1":
		c = SetSourceA1
	case "A2":
		c = SetSourceA2
	case "A3":
		c = SetSourceA3
	case "A4":
		c = SetSourceA4
	case "D1":
		c = SetSourceD1
	case "D2":
		c = SetSourceD2
	case "D3":
		c = SetSourceD3
	case "MP3":
		c = SetSourceMP3
	case "Bluetooth":
		c = SetSourceBluetooth
	case "USB":
		c = SetSourceUSBAudio
	case "A1 Balanced":
		c = SetSourceA1Balanced
	default:
		http.Error(w, fmt.Sprintf("Unknown source: %s", req.Source), http.StatusBadRequest)
	}

	err = a.SendCommand(c)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func main() {
	var wg sync.WaitGroup

	mux := http.NewServeMux()

	amp, err := NewAmplifier("/dev/ttyUSB1")
	if err != nil {
		log.Fatal(err)
	}
	defer amp.port.Close()

	// Get initial state.
	err = amp.SendCommand(GetPowerState)
	if err != nil {
		log.Fatal(err)
	}
	err = amp.SendCommand(GetMuteState)
	if err != nil {
		log.Fatal(err)
	}
	err = amp.SendCommand(GetSource)
	if err != nil {
		log.Fatal(err)
	}

	wg.Add(1)
	go amp.Listen()

	mux.Handle("/status", amp)

	mux.HandleFunc("/source", func(w http.ResponseWriter, r *http.Request) {
		amp.HTTPSource(w, r)
	})
	log.Fatal(http.ListenAndServe(":8080", mux))

	wg.Wait()
}
