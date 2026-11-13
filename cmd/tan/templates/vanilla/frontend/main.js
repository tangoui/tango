// window.tango is injected by the app (runtimejs.Script) before this page loads.
const form = document.getElementById("greet");
const input = document.getElementById("name");
const out = document.getElementById("out");

form.addEventListener("submit", async (e) => {
  e.preventDefault();
  try {
    out.textContent = await window.tango.call("Greeter.Hello", input.value);
  } catch (err) {
    out.textContent = "Error: " + err.message;
  }
});
