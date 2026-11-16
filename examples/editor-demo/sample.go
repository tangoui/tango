package main

import "os"

// startupDocument leaves benchmark generation opt-in without changing its data.
func startupDocument(file string, generatedLines int, explicitGenerator bool) (string, string, error) {
	if file != "" {
		b, err := os.ReadFile(file)
		return file, string(b), err
	}
	if explicitGenerator {
		return "generated.ts", genTS(generatedLines), nil
	}
	return "welcome.ts", welcomeSample, nil
}

const welcomeSample = `// Welcome to Tango
// A native code editor, built entirely in Go.
// Edit this example, then use File > Save As to keep your changes.

type Task = {
  title: string;
  done: boolean;
};

const tasks: Task[] = [
  { title: "Explore the editor", done: true },
  { title: "Try Find and Replace", done: false },
  { title: "Open your own file", done: false },
];

function progress(items: Task[]): string {
  const completed = items.filter((item) => item.done).length;
  return completed + " of " + items.length + " tasks complete";
}

function completeTask(items: Task[], title: string): Task[] {
  return items.map((item) =>
    item.title === title ? { ...item, done: true } : item,
  );
}

const updated = completeTask(tasks, "Try Find and Replace");
console.log(progress(updated));

// File, Edit and View menus contain the available actions.
// Use View for themes and layout; Explorer opens files from your folder.
// Your unsaved edits stay protected when opening a file or quitting.
`
