package testutil

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// WaitForElementText waits for an element to contain specific text
func WaitForElementText(ctx context.Context, selector string, expectedText string) error {
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var text string
			if err := chromedp.Text(selector, &text).Do(ctx); err != nil {
				return err
			}
			if !strings.Contains(text, expectedText) {
				return fmt.Errorf("element text does not contain '%s', got '%s'", expectedText, text)
			}
			return nil
		}),
	)
}

// GetElementText retrieves the text content of an element
func GetElementText(ctx context.Context, selector string) (string, error) {
	var text string
	err := chromedp.Run(ctx,
		chromedp.Text(selector, &text),
	)
	return text, err
}

// ClickElement clicks an element and waits for potential UI updates
func ClickElement(ctx context.Context, selector string) error {
	return chromedp.Run(ctx,
		chromedp.Click(selector),
	)
}

// FillInput fills an input field with text
func FillInput(ctx context.Context, selector string, text string) error {
	return chromedp.Run(ctx,
		chromedp.Focus(selector),
		chromedp.SendKeys(selector, text),
	)
}

// IsElementVisible checks if an element is visible in the DOM
func IsElementVisible(ctx context.Context, selector string) (bool, error) {
	var visible bool
	err := chromedp.Run(ctx,
		chromedp.Evaluate(
			fmt.Sprintf(`document.querySelector('%s') !== null`, selector),
			&visible,
		),
	)
	return visible, err
}

// WaitForPageLoad waits for the DOM to be ready with multiple checks
func WaitForPageLoad(ctx context.Context) error {
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var ready bool
			for i := 0; i < 40; i++ {
				if err := chromedp.Evaluate(`document.readyState === 'complete'`, &ready).Do(ctx); err != nil {
					return err
				}
				if ready {
					time.Sleep(500 * time.Millisecond)
					return nil
				}
				time.Sleep(250 * time.Millisecond)
			}
			return errors.New("page did not reach complete state")
		}),
	)
}

// WaitForWebSocketConnection waits for the WebSocket to connect
func WaitForWebSocketConnection(ctx context.Context) error {
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			for i := 0; i < 40; i++ {
				var wsConnected bool
				err := chromedp.Evaluate(`typeof ws !== 'undefined' && ws.readyState === WebSocket.OPEN`, &wsConnected).Do(ctx)
				if err == nil && wsConnected {
					time.Sleep(500 * time.Millisecond)
					return nil
				}
				time.Sleep(250 * time.Millisecond)
			}
			return errors.New("WebSocket did not connect in time")
		}),
	)
}

// WaitForElement waits for an element to exist and optionally be visible
func WaitForElement(ctx context.Context, selector string, checkVisibility bool) error {
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			for i := 0; i < 40; i++ {
				var exists bool

				if err := chromedp.Evaluate(fmt.Sprintf(`document.querySelector('%s') !== null`, selector), &exists).Do(ctx); err != nil {
					return err
				}

				if exists && !checkVisibility {
					return nil
				}

				if exists && checkVisibility {
					var visible bool
					if err := chromedp.Evaluate(fmt.Sprintf(`
						var elem = document.querySelector('%s');
						elem && elem.offsetParent !== null && window.getComputedStyle(elem).display !== 'none'
					`, selector), &visible).Do(ctx); err != nil {
						return err
					}

					if visible {
						return nil
					}
				}

				time.Sleep(250 * time.Millisecond)
			}
			return fmt.Errorf("element %s not found or not visible after timeout", selector)
		}),
	)
}

// WaitForModal waits for a modal dialog to appear and be visible
func WaitForModal(ctx context.Context) error {
	return WaitForElement(ctx, "#fileModal", true)
}

// CloseModal closes the file management modal by clicking the close button
func CloseModal(ctx context.Context) error {
	return chromedp.Run(ctx,
		chromedp.Click("#closeFileModal"),
		chromedp.Sleep(300*time.Millisecond),
	)
}

// OpenFileModal opens the file management modal by clicking the Files button
func OpenFileModal(ctx context.Context) error {
	return chromedp.Run(ctx,
		chromedp.Click("#filesBtn"),
		chromedp.Sleep(1*time.Second),
	)
}

// SelectFileCheckbox selects or deselects a file checkbox by row index and type (in/out)
func SelectFileCheckbox(ctx context.Context, rowIndex int, checkboxType string) error {
	if checkboxType != "in" && checkboxType != "out" {
		return errors.New("checkboxType must be 'in' or 'out'")
	}

	selector := fmt.Sprintf("#fileList tr:nth-child(%d) input.file%s", rowIndex, checkboxType)
	return chromedp.Run(ctx,
		chromedp.Click(selector),
	)
}

// GetFileListRows returns the number of file rows in the file list table
func GetFileListRows(ctx context.Context) (int, error) {
	var count int
	err := chromedp.Run(ctx,
		chromedp.Evaluate(
			`document.querySelectorAll("#fileList tr").length`,
			&count,
		),
	)
	return count, err
}

// WaitForWebSocketMessage waits for a WebSocket message to arrive
func WaitForWebSocketMessage(ctx context.Context) error {
	return chromedp.Run(ctx,
		chromedp.Sleep(500*time.Millisecond),
	)
}

// GetSelectedFiles returns the currently selected input and output files from the file list
func GetSelectedFiles(ctx context.Context) (inputFiles []string, outputFiles []string, err error) {
	var result map[string]interface{}
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`
			(function() {
				var inputFiles = [];
				var outFiles = [];
				var rows = document.getElementById("fileList").getElementsByTagName("tr");
				for (var i = 0; i < rows.length; i++) {
					var cells = rows[i].getElementsByTagName("td");
					if (cells.length < 3) continue;
					var inChecked = cells[0].querySelector("input").checked;
					var outChecked = cells[1].querySelector("input").checked;
					var filename = cells[2].textContent;
					if (inChecked) inputFiles.push(filename);
					if (outChecked) outFiles.push(filename);
				}
				return { input: inputFiles, output: outFiles };
			})()
		`, &result),
	)

	if err == nil {
		if inp, ok := result["input"].([]interface{}); ok {
			for i := 0; i < len(inp); i++ {
				if f, ok := inp[i].(string); ok {
					inputFiles = append(inputFiles, f)
				}
			}
		}
		if out, ok := result["output"].([]interface{}); ok {
			for i := 0; i < len(out); i++ {
				if f, ok := out[i].(string); ok {
					outputFiles = append(outputFiles, f)
				}
			}
		}
	}

	return
}

// SubmitQuery submits a query by typing in the input field and pressing Enter
func SubmitQuery(ctx context.Context, query string) error {
	return chromedp.Run(ctx,
		chromedp.Focus("#userInput"),
		chromedp.SendKeys("#userInput", query),
		chromedp.SendKeys("#userInput", "\r"),
	)
}

// GetChatContent retrieves the current content of the chat area
func GetChatContent(ctx context.Context) (string, error) {
	var content string
	err := chromedp.Run(ctx,
		chromedp.Text("#chat", &content),
	)
	return content, err
}

// WaitForChatContent waits for specific content to appear in the chat
func WaitForChatContent(ctx context.Context, expectedText string) error {
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			content, err := GetChatContent(ctx)
			if err != nil {
				return err
			}
			if !strings.Contains(content, expectedText) {
				return fmt.Errorf("chat content does not contain '%s'", expectedText)
			}
			return nil
		}),
	)
}