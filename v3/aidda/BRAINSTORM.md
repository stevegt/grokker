# Brainstorm: Parallelizing the AIDDA Workflow

## Goals

1. **Maximize Parallelism**:
    - Execute as many tasks concurrently as possible, especially API calls and test runs.
    - Prevent users from waiting for sequential steps to complete before initiating the next.

2. **Decentralized Workflow**:
    - Minimize the use of Git’s networking and merge features (`pull`, `push`, `fetch`, `merge`).
    - Encourage the development of a more robust decentralized workflow.

## Proposed 'watch' Subcommand

### Overview

Introduce a `watch` subcommand responsible for orchestrating parallel operations through two background daemons:

- **Watch Daemon (W)**
- **Temporary Repo Daemon (R)**

### Daemons Functionality

#### Watch Daemon (W)

- **File Monitoring**:
    - Watches the `A` prompt file for any changes.
    - On detecting a change:
        - **Stashes Uncommitted Changes** in `A`.
        - Creates a CBOR message with the prompt file contents and the stash commit hash (`S`).
        - Sends this message to `R`, promising to respond to any follow-up queries for `S` and its prerequisites.

- **Message Handling**:
    - Listens for messages from `R`.
    - Upon receiving an **object message** from `R`:
        - Adds the object to `A`.
    - Upon receiving a **commit message** from `R`:
        - Adds the commit objects to `A`.
        - Notifies the user that changes are ready to be merged.
    - Upon receiving an **object request** from `R`:
        - Sends a message to `R` containing the requested objects,
          promising that the objects match the requested hashes.

- **Merging Changes**:
    - The daemon performs the following steps:
        1. **Stash Uncommitted Changes** in `A`.
        2. Apply the changes from the B CBOR message to the working directory, without committing.
        3. Resolve any merge conflicts.
        4. **Run the `test` Subcommand** in `A`.
        5. **Display Test Results** to the user.
        6. **Await User Review**:
            - Waits for the user to review changes.
            - Upon completion, the user notifies the daemon to either:
                - **Commit the Changes** in `A`, or
                - **Reject the Changes** with an updated prompt file.

- **Finalizing Commit**:
    - When `W` receives the notification to commit:
        1. Commits the changes in `A`.
        2. Unstashes any stashed changes.

#### Temporary Repo Daemon (R)

- **Message Listening**:
    - Listens for messages from `W`.

- **Object Handling**:
    - On receiving an **object message** from `W`:
        - Adds the object to `T`.

- **Prompt Processing**:
    - On receiving a **prompt message**:
        1. **Check for Commit Hash (`S`) in `T`**:
            - **If `S` is not found**:
                - Sends a message to `W` with the latest commit hash in `T`, promising to generate code from the prompt.
                - Requests the contents of the objects that make up `S` and its prerequisites.
                - Re-adds the message to the tail of the queue.
                - Sleeps for a short duration before re-checking the queue.
            - **If `S` is found**:
                1. Checks out `S` into a new branch (`B`).
                2. Runs the `generate` subcommand in `T` based on the queued prompt file contents.
                3. Runs the `test` subcommand in `T`.
                4. **Test Outcomes**:
                    - **If tests pass**:
                        - Commits the changes in `T` to branch `B`.
                        - Sends a message to `W` containing the commit hash of `B`, promising that the changes satisfy the prompt.
                    - **If tests fail**:
                        - Appends the test output to the prompt file.
                        - Runs the `generate` subcommand in `T` again.
                        - Repeats the process.

### User Notification & Merging Process

1. **Receiving Notification**:
    - When the user gets notified that changes are ready to be merged, they somehow inform `W` to initiate the merge.

## Considerations

- **Concurrency Management**:
    - Ensure thread-safe operations within daemons to handle concurrent messages and file operations.
  
- **Error Handling**:
    - Implement robust error handling for network failures, merge conflicts, and test failures.
  
- **User Experience**:
    - Provide clear and actionable notifications to the user.
    - Ensure minimal disruption to the user’s workflow during automated operations.

- **Scalability**:
    - Design the system to handle large projects with numerous files and complex dependencies.

- **Security**:
    - Secure inter-daemon communications to prevent unauthorized access or malicious interventions.

## Future Enhancements

- **Configurable Parallelism**:
    - Allow users to configure the degree of parallelism based on system resources.
  
- **Logging & Monitoring**:
    - Implement comprehensive logging for auditing and troubleshooting.
    - Provide monitoring tools to visualize the workflow and daemon activities.

- **Extensibility**:
    - Design the architecture to accommodate additional daemons or workflow steps as needed.

