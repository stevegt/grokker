See the end of README-neovim.md for example usage.  This file is for
setting up similar keybindings in Visual Studio Code.

In Visual Studio Code (VS Code), create a custom keybinding and defining a task to run the external command. Here's how you can do it:

1. **Create a Task**: Define a task that runs your external command.
   
   - Open your command palette (`Ctrl+Shift+P` or `Cmd+Shift+P` on macOS) and type "Tasks: Open User Tasks" and select it.
   - Add a task to the `tasks.json` file. Here is an example of how it could look:

     ```json
     {
       "version": "2.0.0",
       "tasks": [
         {
           "label": "Run Grok AIDDA commit and prompt",
           "type": "shell",
           "command": "grok aidda commit prompt",
           "group": {
             "kind": "build",
             "isDefault": true
           },
           "problemMatcher": []
         }
       ]
     }
     ```

2. **Create a Keybinding**: Bind a key to your custom task.

   - Open your command palette (`Ctrl+Shift+P` or `Cmd+Shift+P` on macOS) and type "Preferences: Open Keyboard Shortcuts (JSON)" and select it.
   - Add a keybinding to the `keybindings.json` file. Here is an example:

     ```json
     [
       {
         "key": "ctrl+alt+v",
         "command": "workbench.action.tasks.runTask",
         "args": "Run Grok AIDDA commit and prompt",
       }
     ]
     ```

In this example:
- `"ctrl+alt+v"` is the key combination that triggers the command. You can change it to any key combination you prefer.
- `"Run Grok AIDDA commit and prompt"` is the label of the task defined in `tasks.json`.

After completing these steps, pressing the specified key combination will execute the external command defined in the task.

You can also define workspace-specific tasks and keybindings if you want the customization to apply only to a particular project. To do this, create a `tasks.json` file in the `.vscode` folder of your workspace and define the task there. Then, add the keybinding to your workspace's keybindings file.
