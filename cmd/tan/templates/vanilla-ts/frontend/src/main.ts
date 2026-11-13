import { call } from "@tangoui/runtime";
import "./style.css";

const form = document.querySelector<HTMLFormElement>("#greet")!;
const input = document.querySelector<HTMLInputElement>("#name")!;
const out = document.querySelector<HTMLParagraphElement>("#out")!;

form.addEventListener("submit", async (e) => {
  e.preventDefault();
  try {
    out.textContent = await call<string>("Greeter.Hello", input.value);
  } catch (err) {
    out.textContent = "Error: " + (err as Error).message;
  }
});
