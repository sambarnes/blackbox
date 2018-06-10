package main

import (
	"bytes"
	"crypto/sha256"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	ed "github.com/FactomProject/ed25519"
	"github.com/FactomProject/factom"
	"github.com/dhowden/raspicam"
	"github.com/sambarnes/elmobd"
)

// Types

type Person struct {
	ecAddress *factom.ECAddress // the identity of a user (also used for chain payments)
	chainID   string            // their identity chain to hold vehicle registrations
	vehicles  []Vehicle
	tickets   []Ticket
}

type Vehicle struct {
	vin            string   // the VIN number used as the vehicle's ID
	chainID        string   // the chain holding all dataPointEntries
	owner          *Person  // current owner
	previousOwners [][]byte // public keys of previous owners
}

type Ticket struct {
	// TODO
}

// Functions

// constructChainID takes the ChainName as a string array and returns its ChainID
// see: https://www.factom.com/devs/docs/guide/factom-data-structures
func constructChainID(chainName [][]byte) string {
	e := factom.Entry{}
	for _, nameSegment := range chainName {
		e.ExtIDs = append(e.ExtIDs, nameSegment)
	}
	chain := factom.NewChain(&e)
	return chain.ChainID
}

/*
 * Person functions
 */

// NewPerson creates a new Person using the ecAddress as payment and identity
func NewPerson(ecAddress *factom.ECAddress) *Person {
	var p Person
	p.ecAddress = ecAddress
	chainName := [][]byte{[]byte("Driver Identity Chain"), ecAddress.PubBytes()}
	p.chainID = constructChainID(chainName)
	return &p
}

// IsRegistered returns true if the person's chainID has been registered
func (person *Person) IsRegistered() bool {
	return factom.ChainExists(person.chainID)
}

// Register will try to create a factom chain for the person and return the txID
// ExtIDs = [0]:"Driver Identity Chain", [1]:public key in binary
func (person *Person) Register(ecAddress *factom.ECAddress) (string, error) {
	if person.IsRegistered() {
		return "", nil
	}
	chainEntry := factom.Entry{}
	chainEntry.ExtIDs = [][]byte{[]byte("Driver Identity Chain"), person.ecAddress.PubBytes()}
	chain := factom.NewChain(&chainEntry)
	txID, err := factom.CommitChain(chain, ecAddress)
	if err != nil {
		return "", err
	}
	if _, err := factom.RevealChain(chain); err != nil {
		return "", err
	}
	return txID, nil
}

// InitiateVehicleTransaction lets person sign a message saying that they would like to
// transfer ownership to otherPerson
func (person *Person) InitiateVehicleTransaction(vehicle Vehicle, otherPerson Person) {
	// TODO
}

// ConfirmVehicleTransaction lets person sign a message saying that they would like
// to confirm a previously initiated vehicle transaction
func (person *Person) ConfirmVehicleTransaction(vehicle Vehicle) {
	// TODO
}

/*
 * Vehicle functions
 */

// NewVehicle creates a Vehicle using the given VIN number
func NewVehicle(vin string) *Vehicle {
	if len(vin) != 17 {
		return nil
	}

	var v Vehicle
	v.vin = vin
	chainName := [][]byte{[]byte("Vehicle Identity Chain"), []byte(vin)}
	v.chainID = constructChainID(chainName)
	return &v
}

// IsRegistered returns true if the vehicle's chainID has been registered
func (vehicle *Vehicle) IsRegistered() bool {
	return factom.ChainExists(vehicle.chainID)
}

// Register will try to create a factom chain for the vehicle and return the txID
// ExtIDs = [0]:"Vehicle Identity Chain", [1]:string of vin number
func (vehicle *Vehicle) Register(ecAddress *factom.ECAddress) (string, error) {
	if vehicle.IsRegistered() {
		return "", nil
	}
	chainEntry := factom.Entry{}
	chainEntry.ExtIDs = [][]byte{[]byte("Vehicle Identity Chain"), []byte(vehicle.vin)}
	chain := factom.NewChain(&chainEntry)
	txID, err := factom.CommitChain(chain, ecAddress)
	if err != nil {
		return "", err
	}
	if _, err := factom.RevealChain(chain); err != nil {
		return "", err
	}
	return txID, nil
}

func (vehicle *Vehicle) StartRecording() {
	vehicle.RecordOBD()
	// go vehicle.RecordVideo()
}

// RecordVideo begins recording with the raspberry pi camera module,
// hashes segments of video at the given interval in seconds,
// and commits that hash to the camera's chain
func (vehicle *Vehicle) RecordVideo(interval int) {
	fmt.Println("Recording started...")

	for i := 0; i < 5; i++ {
		fmt.Println("Capturing video...")

		videoPath, _ := vehicle.captureVideoSegment(interval)
		hash, _ := vehicle.getFileHash(videoPath)

		fmt.Printf("Video saved at %s with hash %s", videoPath, hash)

		vehicle.secureHashOnChain(hash)
	}
}

// RecordOBD begins logging
func (vehicle *Vehicle) RecordOBD() {
	// TODO: use a real device, not just a mock
	serialPath := flag.String(
		"serial",
		"/dev/ttyUSB0",
		"Path to the serial device to use",
	)
	flag.Parse()

	dev, err := elmobd.NewTestDevice(*serialPath, false)
	if err != nil {
		fmt.Println("Failed to create new device", err)
		return
	}

	for i := 0; i < 1; i++ {
		now := time.Now().Format("20060102150405")
		filepath := fmt.Sprintf("%s.txt", now)
		for j := 0; j < 60; j++ {
			// Run all commands
			timeSinceStart, err := dev.RunOBDCommand(elmobd.NewRuntimeSinceStart())
			speed, err := dev.RunOBDCommand(elmobd.NewVehicleSpeed())
			rpm, err := dev.RunOBDCommand(elmobd.NewEngineRPM())
			throttle, err := dev.RunOBDCommand(elmobd.NewThrottlePosition())
			fuelPressure, err := dev.RunOBDCommand(elmobd.NewFuelPressure())
			timingAdvance, err := dev.RunOBDCommand(elmobd.NewTimingAdvance())
			coolant, err := dev.RunOBDCommand(elmobd.NewCoolantTemperature())
			engineLoad, err := dev.RunOBDCommand(elmobd.NewEngineLoad())
			manifoldPressure, err := dev.RunOBDCommand(elmobd.NewIntakeManifoldPressure())
			maf, err := dev.RunOBDCommand(elmobd.NewMafAirFlowRate())
			shortterm1, err := dev.RunOBDCommand(elmobd.NewShortFuelTrim1())
			shortterm2, err := dev.RunOBDCommand(elmobd.NewShortFuelTrim2())
			longterm1, _ := dev.RunOBDCommand(elmobd.NewLongFuelTrim1())
			longterm2, _ := dev.RunOBDCommand(elmobd.NewLongFuelTrim2())

			// Compile command results
			results := strings.Join([]string{
				fmt.Sprintf("%s/n", time.Now().String()),
				fmt.Sprintf("Runtime Since Start: %s sec", timeSinceStart.ValueAsLit()),
				fmt.Sprintf("Vehichle Speed: %s km/h", speed.ValueAsLit()),
				fmt.Sprintf("Engine RPM: %s", rpm.ValueAsLit()),
				fmt.Sprintf("Throttle Position: %s%%", throttle.ValueAsLit()),
				fmt.Sprintf("Fuel Pressure: %s kPa", fuelPressure.ValueAsLit()),
				fmt.Sprintf("Timing Advance: %s deg before TDC", timingAdvance.ValueAsLit()),
				fmt.Sprintf("Coolant Temp: %s C", coolant.ValueAsLit()),
				fmt.Sprintf("Engine Load: %s%%", engineLoad.ValueAsLit()),
				fmt.Sprintf("Intake Manifold Pressure: %s kPa", manifoldPressure.ValueAsLit()),
				fmt.Sprintf("MAF Air Flow Rate: %s grams/sec", maf.ValueAsLit()),
				fmt.Sprintf("Short Term Fuel Trim 1: %s%%", shortterm1.ValueAsLit()),
				fmt.Sprintf("Short Term Fuel Trim 2: %s%%", shortterm2.ValueAsLit()),
				fmt.Sprintf("Long Term Fuel Trim 1: %s%%", longterm1.ValueAsLit()),
				fmt.Sprintf("Long Term Fuel Trim 2: %s%%", longterm2.ValueAsLit()),
				"------------------------------------------------------------------\n",
			}, "\n")

			// Try to open the current working file
			file, err := os.OpenFile(filepath, os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				// File doesn't exist, create it
				file, err = os.Create(filepath)
				if err != nil {
					panic(err)
				}

				// Write the OBD results
				if _, err = file.WriteString(results); err != nil {
					panic(err)
				}

				file.Close()
				fmt.Println("File created.")
				time.Sleep(1 * time.Second)
				continue
			}

			// File exists
			if _, err = file.WriteString(results); err != nil {
				panic(err)
			}

			file.Close()
			fmt.Println("File has been updated.")
			time.Sleep(1 * time.Second)
		}
		hash, err := vehicle.getFileHash(filepath)
		if err != nil {
			panic(err)
		}
		txID, err := vehicle.secureHashOnChain(hash)
		if err != nil {
			panic(err)
		}
		fmt.Printf("File secured to factom. TxID: %s\n", txID)
	}
}

// VerifyData will check the integrity of a local file
func (vehicle *Vehicle) VerifyData(filepath string) (bool, error) {
	fmt.Println("Verifying started...")
	entries, err := factom.GetAllChainEntries(vehicle.chainID)
	if err != nil {
		return false, err
	}
	localHash, err := vehicle.getFileHash(filepath)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if len(entry.ExtIDs) != 2 {
			continue // invalid ExtID structure
		}
		// check if the pub key matches
		pubKey := entry.ExtIDs[1]
		if bytes.Compare(pubKey, vehicle.owner.ecAddress.PubBytes()) != 0 {
			continue
		}
		// check if the signature is valid
		var signature [64]byte
		copy(signature[:], entry.ExtIDs[0])
		if !ed.Verify(vehicle.owner.ecAddress.PubFixed(), entry.Content, &signature) {
			continue
		}

		// check if localHash is found on-chain
		if bytes.Compare(localHash, entry.Content) == 0 {
			return true, nil
		}
	}
	return false, nil
}

// captureVideoSegment uses the raspicam package to capture a video
// of length <interval> seconds and return its filepath
func (vehicle *Vehicle) captureVideoSegment(interval int) (string, error) {
	// create file for the video
	now := time.Now().Format("20060102150405")
	path := fmt.Sprintf("%s.h264", now)

	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create file: %v", err)
		return "", err
	}
	defer f.Close()

	// capture 10 seconds of video
	s := raspicam.NewVid()
	s.Args = append(s.Args, "-o", path, "-t", "10000")
	errCh := make(chan error)
	go func() {
		for x := range errCh {
			fmt.Fprintf(os.Stderr, "%v\n", x)
		}
	}()

	raspicam.Capture(s, f, errCh)

	return path, nil
}

// getFileHash returns the hash of a file located at path
func (vehicle *Vehicle) getFileHash(path string) ([]byte, error) {
	// get file as a byte array
	file, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// get a fingerprint of the file
	sha := sha256.New()
	sha.Write(file)
	hash := sha.Sum(nil)

	return hash, nil
}

// secureHashOnChain writes the input hash to the Vehicle's chainID along with a
// signature produced by the same entry credit private key used for payment
func (vehicle *Vehicle) secureHashOnChain(hash []byte) (string, error) {
	// signature of the hash will be ExtIDs[0], used for later validation
	signature := ed.Sign(vehicle.owner.ecAddress.Sec, hash)

	entry := factom.Entry{}
	entry.ChainID = vehicle.chainID
	entry.ExtIDs = [][]byte{signature[:], vehicle.owner.ecAddress.PubBytes()}
	entry.Content = []byte(hash)

	txID, err := factom.CommitEntry(&entry, vehicle.owner.ecAddress)
	if err != nil {
		return "", err
	}
	if _, err := factom.RevealEntry(&entry); err != nil {
		return "", err
	}
	return txID, nil
}

// checkFileIntegrity returns true if the file located at filepath hashes
// to the same value that is stored on chain at entryHash
func (vehicle *Vehicle) checkFileIntegrity(filepath string, entryHash string) (bool, error) {
	onDiskHash, err := vehicle.getFileHash(filepath)
	if err != nil {
		return false, err
	}

	entry, err := factom.GetEntry(entryHash)
	if err != nil {
		return false, err
	}

	onChainHash := entry.Content
	var signature [64]byte
	copy(signature[:], entry.ExtIDs[0])

	validSig := ed.Verify(vehicle.owner.ecAddress.Pub, onChainHash, &signature)
	hashComparison := bytes.Compare(onDiskHash, onChainHash)
	if !validSig || hashComparison != 0 {
		return false, nil
	}
	return true, nil
}

// Program Entry Point
func init() {
	// Connect to the courtesy node from Factom Inc.
	factom.SetFactomdServer("courtesy-node.factom.com")
}

func main() {
	// TODO: use proper key management
	ecKey := "PRIVATE KEY HERE"
	ecAddress, err := factom.GetECAddress(ecKey)
	if err != nil {
		panic(err)
	}

	vehicle := NewVehicle("1234567890ABCDEFH")
	if txID, err := vehicle.Register(ecAddress); err != nil {
		panic(err)
	} else if txID == "" {
		fmt.Printf("Vehicle already registered. ChainID: %s\n", vehicle.chainID)
	} else {
		fmt.Printf("Vehicle registered. TxID: %s\n", txID)
	}

	person := NewPerson(ecAddress)
	if txID, err := person.Register(ecAddress); err != nil {
		panic(err)
	} else if txID == "" {
		fmt.Printf("Person already registered. ChainID: %s\n", person.chainID)
	} else {
		fmt.Printf("Person registered. TxID: %s\n", txID)
	}
	vehicle.owner = person
}
