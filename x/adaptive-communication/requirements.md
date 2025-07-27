
Write a Go program that simulates a system of agents that communicate
with each other in an adaptive manner. The agents should be able to
evolve their communication protocols based on the tasks they perform,
discovering new strategies through interaction. The code should be
modular, allowing for easy extension with new agent types and flexible
message parsing and generation.

Each agent has a subdirectory containing its own configuration and
data files, including a JSON configuration file that contains the name
of the LLM model the agent uses as its "brain", a "goal.md" file that
guides the agent's actions, and a "pseudocode.md" file that contains
the algorithms that control the agent's behavior.  An agent is able to
read and write to its own pseudocode file, allowing it to adapt its
algorithms over time, but it is not allowed to modify the "goal.md"
file. An agent cannot see the goals or pseudocode of other agents, but
it can communicate with them via point-to-point messages. 

The main goroutine loads these agents and simulates their
interactions. The agents send and receive messages via the main
goroutine, and adapt their communication strategies based on the
messages they receive and the goals they are trying to reach.  The
main goroutine keeps a log of each message sent and received, along
with the agent IDs involved and the timestamp of the interaction. The
agents should be able to run concurrently, simulating a real-time
system.

The main goroutine exits after a specified timeout period.  

The main goroutine makes LLM API calls via the
"github.com/stevegt/grokker/v3/core' package, specifically the
SendWithFiles and ExtractFiles functions, to interpret the agents'
goals and pseudocode, and to generate messages and new pseudocode. See
aidda.go for examples of how to create a Grokker object and use these
functions.

An agent should keep a log of the messages it has sent and received.
This log should be stored in a file named "messages.log" in the agent's
subdirectory. 

Before an agent sends a message, it should compose the message by
sending its goal.md, pseudocode.md, and messages.log files to the
Grokker API at the given model using the SendWithFiles function. The
Grokker API will then generate a message based on the agent's current
state and the communication protocols it has learned, and the agent
will extract the message from the API response using the ExtractFiles
function. The agent will then send this message to the intended
recipient via the main goroutine.
