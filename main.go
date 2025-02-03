// main.go
//
// This tool spawns "libinput debug-events" and reads raw touch events,
// aggregating them into multi-touch gestures. It relies solely on TOUCH_MOTION
// events (using TOUCH_FRAME boundaries to decide when touches have ended) and
// uses a JSON configuration file to determine which command to run for each gesture
// (e.g. "3swipe_up"). The configuration file is in JSON (default "config.json",
// override with -config or -c).
//
// Usage examples:
//
//	To run normally with a config file:
//	    sudo ./ffgestures -c=config.json
//	To print the version:
//	    ./ffgestures -v
//
// Build with:
//
//	go build -o ffgestures main.go
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ------------------ Logging ------------------

// Log prints a message with the specified level and a timestamp.
// Available levels: "info", "error", "warn", "debug".
// If the level is "debug" and config.Debug is false, the message is suppressed.
func Log(level, msg string) {
	if level == "debug" && !config.Debug {
		return
	}
	switch level {
	case "info":
		fmt.Printf("\x1b[32m%s [INFO] %s\x1b[0m\n", time.Now().Format("15:04:05"), msg)
	case "error":
		fmt.Printf("\x1b[31m%s [ERROR] %s\x1b[0m\n", time.Now().Format("15:04:05"), msg)
	case "warn":
		fmt.Printf("\x1b[33m%s [WARNING] %s\x1b[0m\n", time.Now().Format("15:04:05"), msg)
	case "debug":
		fmt.Printf("\x1b[36m%s [DEBUG] %s\x1b[0m\n", time.Now().Format("15:04:05"), msg)
	default:
		fmt.Printf("%s [UNKNOWN] %s\n", time.Now().Format("15:04:05"), msg)
	}
}

// ------------------ Version ------------------

const version = "ffgestures version 1.0.0"

// ------------------ Configuration ------------------

// Config holds configurable settings.
type Config struct {
	Threshold      float64           `json:"threshold"`
	GestureActions map[string]string `json:"gestureActions"`
	Debug          bool              `json:"debug"`
}

// Global configuration. Defaults are provided and will be overridden
// if a config file is found.
var config = Config{
	Threshold: 10.0,
	GestureActions: map[string]string{
		"3swipe_left":  "echo '3-finger swipe left action executed'",
		"3swipe_right": "echo '3-finger swipe right action executed'",
		"3swipe_up":    "echo '3-finger swipe up action executed'",
		"3swipe_down":  "echo '3-finger swipe down action executed'",
	},
	Debug: true,
}

// ------------------ Touch Tracking ------------------

// TouchPoint holds per-finger state: its starting coordinates and last known coordinates.
type TouchPoint struct {
	id             int
	startX, startY float64
	lastX, lastY   float64
}

// Global state for tracking touches.
var (
	// activeTouches tracks currently active touches by finger ID.
	activeTouches = make(map[int]*TouchPoint)
	// finishedTouchesMap holds finished touches (deduplicated by finger ID).
	finishedTouchesMap = make(map[int]*TouchPoint)
	// currentFrameUpdated tracks which finger IDs updated in the current frame.
	currentFrameUpdated = make(map[int]bool)
)

// ------------------ Event Parsing ------------------

// Regular expressions to parse libinput debug-events output.
// We are only interested in TOUCH_MOTION events.
// Example line:
//
//	" event11  TOUCH_MOTION            +37.797s	1 (1) 26.98/42.53 (61.39/58.07mm)"
var touchEventRegex = regexp.MustCompile(`^\s*(\S+)\s+(TOUCH_MOTION)\s+\+[\d.]+s\s+(\d+)(?:\s+\(\d+\))?(?:\s+([\d.]+)/([\d.]+))?`)

// touchFrameRegex matches TOUCH_FRAME events.
var touchFrameRegex = regexp.MustCompile(`^\s*(\S+)\s+TOUCH_FRAME\s+\+[\d.]+s`)

// ------------------ Main ------------------

func main() {
	// Define flags.
	var configPath string
	flag.StringVar(&configPath, "config", "config.json", "Path to configuration file")
	flag.StringVar(&configPath, "c", "config.json", "Path to configuration file (alias)")
	verFlag := flag.Bool("v", false, "Print version and exit")
	verFlagLong := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	// If version flag is set, print version and exit.
	if *verFlag || *verFlagLong {
		fmt.Println(version)
		os.Exit(0)
	}

	// Check that "libinput" command is available.
	if _, err := exec.LookPath("libinput"); err != nil {
		Log("error", "libinput command not found. Please install libinput before running this tool.")
		os.Exit(1)
	}

	// Load configuration from file if available.
	if file, err := os.Open(configPath); err == nil {
		defer file.Close()
		decoder := json.NewDecoder(file)
		if err := decoder.Decode(&config); err != nil {
			Log("error", fmt.Sprintf("Error decoding config file: %v", err))
		} else {
			Log("info", fmt.Sprintf("Loaded config from %s", configPath))
		}
	} else {
		Log("warn", fmt.Sprintf("Could not open config file %s, using default configuration", configPath))
	}

	if config.Debug {
		Log("debug", "Debug mode is enabled")
	}

	// Start "libinput debug-events" as an external command.
	cmd := exec.Command("libinput", "debug-events")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		Log("error", fmt.Sprintf("Error creating stdout pipe: %v", err))
		os.Exit(1)
	}
	if err := cmd.Start(); err != nil {
		Log("error", fmt.Sprintf("Error starting libinput debug-events: %v", err))
		os.Exit(1)
	}

	// Handle SIGINT/SIGTERM for graceful shutdown.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		Log("info", "Terminating...")
		cmd.Process.Kill()
		os.Exit(0)
	}()

	// Process libinput output line by line.
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if config.Debug {
			Log("debug", fmt.Sprintf("Raw line: %s", line))
		}
		processLine(line)
	}
	if err := scanner.Err(); err != nil {
		Log("error", fmt.Sprintf("Error reading libinput output: %v", err))
		os.Exit(1)
	}
	if err := cmd.Wait(); err != nil {
		Log("warn", fmt.Sprintf("libinput debug-events terminated with error: %v", err))
	}
}

// ------------------ Event Handlers ------------------

// processLine handles a single line from libinput.
// We only process TOUCH_MOTION events; TOUCH_FRAME events are handled separately.
func processLine(line string) {
	// Check if this is a TOUCH_FRAME event.
	if touchFrameRegex.MatchString(line) {
		Log("debug", "Detected TOUCH_FRAME event")
		processFrame()
		return
	}

	// Attempt to match a TOUCH_MOTION event.
	matches := touchEventRegex.FindStringSubmatch(line)
	if len(matches) == 0 {
		Log("debug", fmt.Sprintf("Line did not match any known pattern: %s", line))
		return
	}

	fingerID, err := strconv.Atoi(matches[3])
	if err != nil {
		Log("error", fmt.Sprintf("Error parsing finger ID: %v", err))
		return
	}

	// Parse coordinate values.
	var x, y float64
	if len(matches) >= 6 && matches[4] != "" && matches[5] != "" {
		x, err = strconv.ParseFloat(matches[4], 64)
		if err != nil {
			Log("error", fmt.Sprintf("Error parsing x coordinate: %v", err))
		}
		y, err = strconv.ParseFloat(matches[5], 64)
		if err != nil {
			Log("error", fmt.Sprintf("Error parsing y coordinate: %v", err))
		}
	}

	// Mark that this finger updated during the current frame.
	currentFrameUpdated[fingerID] = true

	// Process the TOUCH_MOTION event.
	// If the finger is not already active, create a new record using the current coordinates.
	if tp, exists := activeTouches[fingerID]; exists {
		tp.lastX = x
		tp.lastY = y
		Log("debug", fmt.Sprintf("TOUCH_MOTION: finger %d moved to (%.2f, %.2f)", fingerID, x, y))
	} else {
		tp := &TouchPoint{
			id:     fingerID,
			startX: x,
			startY: y,
			lastX:  x,
			lastY:  y,
		}
		activeTouches[fingerID] = tp
		Log("debug", fmt.Sprintf("TOUCH_MOTION (new): finger %d at (%.2f, %.2f)", fingerID, x, y))
	}
}

// processFrame is called whenever a TOUCH_FRAME event is received.
// It assumes that any active touch that did not update during the current frame
// has been lifted.
func processFrame() {
	// For each active touch not updated in this frame, mark it as finished.
	for fingerID, tp := range activeTouches {
		if _, updated := currentFrameUpdated[fingerID]; !updated {
			finishedTouchesMap[fingerID] = tp
			delete(activeTouches, fingerID)
			Log("debug", fmt.Sprintf("Assuming finger %d lifted (no update in frame)", fingerID))
		}
	}
	// Clear the update tracker for the next frame.
	currentFrameUpdated = make(map[int]bool)

	// When there are no active touches and we have finished touches, process the gesture.
	if len(activeTouches) == 0 && len(finishedTouchesMap) > 0 {
		var finishedTouches []*TouchPoint
		for _, tp := range finishedTouchesMap {
			finishedTouches = append(finishedTouches, tp)
		}
		processGesture(finishedTouches)
		// Reset finished touches map for the next gesture.
		finishedTouchesMap = make(map[int]*TouchPoint)
	}
}

// processGesture computes the overall movement based on the finished touches.
// It averages the deltas (last - start) for each finger and, if the movement
// exceeds the threshold, determines the dominant swipe direction and executes
// the corresponding command from the config.
func processGesture(touches []*TouchPoint) {
	count := len(touches)
	var totalDx, totalDy float64
	for _, tp := range touches {
		dx := tp.lastX - tp.startX
		dy := tp.lastY - tp.startY
		totalDx += dx
		totalDy += dy
	}
	avgDx := totalDx / float64(count)
	avgDy := totalDy / float64(count)
	Log("info", fmt.Sprintf("Gesture completed with %d finger(s): avg dx=%.2f, avg dy=%.2f", count, avgDx, avgDy))

	// Ignore minor movements.
	if math.Abs(avgDx) < config.Threshold && math.Abs(avgDy) < config.Threshold {
		Log("debug", "Movement below threshold, gesture ignored")
		return
	}

	// Determine the dominant swipe direction.
	var direction string
	if math.Abs(avgDx) > math.Abs(avgDy) {
		if avgDx > 0 {
			direction = "right"
		} else {
			direction = "left"
		}
	} else {
		if avgDy > 0 {
			direction = "down"
		} else {
			direction = "up"
		}
	}
	gestureKey := fmt.Sprintf("%dswipe_%s", count, direction)
	Log("info", fmt.Sprintf("Detected gesture: %s", gestureKey))
	if cmdStr, exists := config.GestureActions[gestureKey]; exists {
		go executeCommand(cmdStr)
	} else {
		Log("warn", fmt.Sprintf("No action mapped for gesture: %s", gestureKey))
	}
}

// executeCommand runs the provided shell command using "sh -c" and logs its output.
// The command inherits the environment so that variables like XDG_RUNTIME_DIR are preserved.
func executeCommand(command string) {
	Log("info", fmt.Sprintf("Executing command: %s", command))
	cmd := exec.Command("sh", "-c", command)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		Log("error", fmt.Sprintf("Error executing command: %v\nOutput: %s", err, strings.TrimSpace(string(output))))
	} else {
		Log("debug", fmt.Sprintf("Command output: %s", strings.TrimSpace(string(output))))
	}
}
