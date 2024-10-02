package main

import (
	"fmt"
	"log"
	"regexp"

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

// Reply represents a reply from the CXA amplifier.
type Reply struct {
	Group  string
	Number string
	Data   string
}

var validReply = regexp.MustCompile(`^#(\d\d),(\d\d)(?:,(.*))?\r$`)

func (r *Reply) String() string {
	desc := "Unknown reply"

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
		case "02":
			desc = "Current mute state"
		}
	case "04":
		if r.Number == "01" {
			desc = "Current source"
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

// Amplifier represents the CXA amplifier and its serial connection.
type Amplifier struct {
	port serial.Port
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
func (a *Amplifier) Read() (*Reply, error) {
	buf := make([]byte, 1024)
	n, err := a.port.Read(buf)
	if err != nil {
		return nil, err
	}

	response := string(buf[:n])
	matches := validReply.FindStringSubmatch(response)
	if matches == nil {
		return nil, fmt.Errorf("invalid reply format: %s", response)
	}

	reply := &Reply{
		Group:  matches[1],
		Number: matches[2],
	}
	// If data is present, capture it
	if len(matches) > 3 {
		reply.Data = matches[3]
	}

	return reply, nil
}

func main() {
	amp, err := NewAmplifier("/dev/ttyUSB1")
	if err != nil {
		log.Fatal(err)
	}
	defer amp.port.Close()

	// Get current power state
	err = amp.SendCommand(GetPowerState)
	if err != nil {
		log.Fatal(err)
	}
	r, err := amp.Read()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Response: %v", r)

	// Get current power state
	err = amp.SendCommand(SetPowerOn)
	if err != nil {
		log.Fatal(err)
	}
	r, err = amp.Read()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Response: %v", r)

	// Get current power state
	err = amp.SendCommand(GetPowerState)
	if err != nil {
		log.Fatal(err)
	}
	r, err = amp.Read()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Response: %v", r)

	// Get current source
	err = amp.SendCommand(GetSource)
	if err != nil {
		log.Fatal(err)
	}
	r, err = amp.Read()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Response: %v", r)
}
