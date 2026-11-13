import { call, emit, on } from "@tangoui/runtime";
import "./style.css";

const $ = <T extends HTMLElement>(sel: string) => document.querySelector<T>(sel)!;

// Page → panel: every click is sent to Go as the "page:clicked" event.
let clicks = 0;
$("#count").addEventListener("click", () => {
  clicks++;
  $("#clicks").textContent = String(clicks);
  emit("page:clicked", clicks);
});

// Panel → page: the panel's buttons emit these events from Go.
on("panel:message", (text: string) => {
  $("#from-panel").textContent = text;
});
on("panel:reset", () => {
  clicks = 0;
  $("#clicks").textContent = "0";
});

// An ordinary call to a Go method.
$<HTMLFormElement>("#greet").addEventListener("submit", async (e) => {
  e.preventDefault();
  const name = $<HTMLInputElement>("#name").value;
  try {
    $("#greeting").textContent = await call<string>("Greeter.Hello", name);
  } catch (err) {
    $("#greeting").textContent = "Error: " + (err as Error).message;
  }
});

// Some content to scroll under the panel.
const filler = $("#filler");
for (let i = 1; i <= 20; i++) {
  const p = document.createElement("p");
  p.className = "card";
  p.textContent = `Paragraph ${i}: scroll and select this text, even next to the panel.`;
  filler.appendChild(p);
}
