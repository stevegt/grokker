package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AgentConfig holds configuration for an agent including its LLM brain,
// goal file, and pseudocode file.
type AgentConfig struct {
	Brain          string `json:"brain"`
	GoalFile       string `json:"goal_md"`
	PseudocodeFile string `json:"pseudocode_md"`
}

// Message represents a point-to-point message exchanged between agents.
type Message struct {
	SrcID     string
	DestID    string
	Content   string
	Timestamp time.Time
}

// Agent represents an agent in the simulation.
// Each agent holds its configuration, channels for incoming and outgoing
// messages, a stop channel, and its current pseudocode (as an adaptive
// communication protocol).
type Agent struct {
	ID         string
	Config     AgentConfig
	Incoming   chan Message
	Outgoing   chan Message
	Stop       chan bool
	PseudoMux  sync.Mutex
	Pseudocode string
}

// NewAgent creates a new Agent by reading its configuration from the given
// directory. The configuration is expected to be in a JSON file named
// "config.json" in the agent's subdirectory.
func NewAgent(id, dir string) (*Agent, error) {
	configPath := filepath.Join(dir, "config.json")
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("error reading config file for agent %s: %v",
			id, err)
	}
	var cfg AgentConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config for agent %s: %v",
			id, err)
	}
	// Load initial pseudocode from the pseudocode file.
	pseudoPath := filepath.Join(dir, cfg.PseudocodeFile)
	pseudoData, err := ioutil.ReadFile(pseudoPath)
	if err != nil {
		// If file does not exist, start with a default pseudocode.
		pseudoData = []byte("initial pseudocode")
	}
	agent := &Agent{
		ID:         id,
		Config:     cfg,
		Incoming:   make(chan Message, 10),
		Outgoing:   make(chan Message, 10),
		Stop:       make(chan bool),
		Pseudocode: string(pseudoData),
	}
	return agent, nil
}

// Run starts the agent's main loop. The agent processes incoming messages
// and occasionally sends messages to randomly selected peers. Upon receiving
// a message, the agent adapts its pseudocode incrementally.
func (a *Agent) Run(allAgents map[string]*Agent, wg *sync.WaitGroup) {
	defer wg.Done()
	// Seed random number generator using unique agent data.
	rand.Seed(time.Now().UnixNano() + int64(len(a.ID)))
	for {
		select {
		case msg := <-a.Incoming:
			log.Printf("Agent %s received message from %s: %s",
				a.ID, msg.SrcID, msg.Content)
			a.adaptProtocol(msg)
		case <-time.After(time.Duration(rand.Intn(3000)+1000) *
			time.Millisecond):
			a.sendMessage(allAgents)
		case <-a.Stop:
			log.Printf("Agent %s stopping.", a.ID)
			return
		}
	}
}

// sendMessage constructs a message using the agent's current pseudocode and
// sends it to a randomly selected peer.
func (a *Agent) sendMessage(allAgents map[string]*Agent) {
	// Skip sending if there is only one agent.
	if len(allAgents) <= 1 {
		return
	}
	recipientIDs := []string{}
	for id := range allAgents {
		if id != a.ID {
			recipientIDs = append(recipientIDs, id)
		}
	}
	destID := recipientIDs[rand.Intn(len(recipientIDs))]
	a.PseudoMux.Lock()
	baseProto := a.Pseudocode
	a.PseudoMux.Unlock()
	content := fmt.Sprintf("Protocol v* [%s] message at %s",
		baseProto, time.Now().Format(time.RFC3339))
	msg := Message{
		SrcID:     a.ID,
		DestID:    destID,
		Content:   content,
		Timestamp: time.Now(),
	}
	// Send the constructed message to the broker via the outgoing channel.
	a.Outgoing <- msg
	log.Printf("Agent %s sent message to %s.", a.ID, destID)
	a.adaptAfterSending(msg)
}

// adaptProtocol simulates protocol adaptation by appending a note to the
// pseudocode based on the received message.
func (a *Agent) adaptProtocol(msg Message) {
	a.PseudoMux.Lock()
	defer a.PseudoMux.Unlock()
	update := fmt.Sprintf(" | adapted on receive from %s", msg.SrcID)
	a.Pseudocode += update
	// Write the updated pseudocode back to the pseudocode file.
	pseudoFile := filepath.Join(filepath.Dir(a.Config.PseudocodeFile),
		a.Config.PseudocodeFile)
	err := ioutil.WriteFile(pseudoFile, []byte(a.Pseudocode), 0644)
	if err != nil {
		log.Printf("Agent %s failed to write pseudocode: %v", a.ID, err)
	}
}

// adaptAfterSending simulates a slight adaptation in the pseudocode after
// sending a message.
func (a *Agent) adaptAfterSending(msg Message) {
	a.PseudoMux.Lock()
	defer a.PseudoMux.Unlock()
	update := fmt.Sprintf(" | sent update at %s", time.Now().Format(
		time.RFC3339))
	a.Pseudocode += update
	pseudoFile := filepath.Join(filepath.Dir(a.Config.PseudocodeFile),
		a.Config.PseudocodeFile)
	err := ioutil.WriteFile(pseudoFile, []byte(a.Pseudocode), 0644)
	if err != nil {
		log.Printf("Agent %s failed to write pseudocode: %v", a.ID, err)
	}
}

// Broker routes messages from agents to their intended recipients.
// It listens on a central broker channel and delivers messages to agents'
 // incoming channels.
func Broker(agents map[string]*Agent, brokerCh chan Message,
	stopCh chan bool, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case msg := <-brokerCh:
			if recipient, ok := agents[msg.DestID]; ok {
				recipient.Incoming <- msg
			}
		case <-stopCh:
			log.Println("Broker stopping.")
			return
		}
	}
}

// LoadAgents scans the specified parent directory for subdirectories.
// Each subdirectory is assumed to represent an agent and must contain a
// "config.json" file.
func LoadAgents(agentsDir string) (map[string]*Agent, error) {
	agents := make(map[string]*Agent)
	entries, err := ioutil.ReadDir(agentsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read agents directory: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			agentID := entry.Name()
			agentPath := filepath.Join(agentsDir, agentID)
			agent, err := NewAgent(agentID, agentPath)
			if err != nil {
				log.Printf("Skipping agent %s: %v", agentID, err)
				continue
			}
			agents[agentID] = agent
		}
	}
	return agents, nil
}

func main() {
	// Use "./agents" as the default agents directory.
	agentsDir := "./agents"
	if len(os.Args) > 1 {
		agentsDir = os.Args[1]
	}
	agents, err := LoadAgents(agentsDir)
	if err != nil {
		log.Fatalf("Error loading agents: %v", err)
	}
	if len(agents) == 0 {
		log.Fatalf("No agents found in directory: %s", agentsDir)
	}
	log.Printf("Loaded %d agents.", len(agents))
	// Central broker channel for message passing.
	brokerCh := make(chan Message, 50)
	brokerStop := make(chan bool)
	var wg sync.WaitGroup

	// Start the broker goroutine.
	wg.Add(1)
	go Broker(agents, brokerCh, brokerStop, &wg)

	// Start each agent's goroutine and forward its outgoing messages
	for _, agent := range agents {
		go func(a *Agent) {
			for msg := range a.Outgoing {
				brokerCh <- msg
			}
		}(agent)
		wg.Add(1)
		go agent.Run(agents, &wg)
	}

	// Run the simulation for a fixed timeout period.
	simDuration := 10 * time.Second
	log.Printf("Starting simulation for %v...", simDuration)
	time.Sleep(simDuration)

	// Signal all agents and the broker to stop.
	for _, agent := range agents {
		agent.Stop <- true
	}
	brokerStop <- true

	// Allow goroutines a moment to finish.
	time.Sleep(1 * time.Second)
	log.Println("Simulation ended. Messages were logged to stdout.")
	wg.Wait()
}
