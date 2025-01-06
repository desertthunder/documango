function initializeTheme() {
  const savedTheme = localStorage.getItem("theme");
  const prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
  const initialTheme = savedTheme || (prefersDark ? "dark" : "light");

  document.documentElement.setAttribute("data-theme", initialTheme);
  localStorage.setItem("theme", initialTheme);

  window
    .matchMedia("(prefers-color-scheme: dark)")
    .addEventListener("change", (e) => {
      if (!localStorage.getItem("theme")) {
        document.documentElement.setAttribute(
          "data-theme",
          e.matches ? "dark" : "light"
        );
      }
    });

  setThemeName();
}

function toggle() {
  const currentTheme = localStorage.getItem("theme") || "light";
  const newTheme = currentTheme === "light" ? "dark" : "light";
  document.documentElement.setAttribute("data-theme", newTheme);
  localStorage.setItem("theme", newTheme);

  setThemeName();
}

function setThemeName() {
  const theme = document.documentElement.dataset["theme"];
  const toggler = document.querySelector("[data-toggle]");
  toggler.textContent = theme === "dark" ? "Lighten" : "Darken";
}

(function init() {
  const toggler = document.querySelector("[data-toggle]");
  if (toggler) {
    toggler.addEventListener("click", toggle);
  }

  initializeTheme();
})()
