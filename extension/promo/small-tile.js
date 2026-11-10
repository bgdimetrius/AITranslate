const creative = {
  brand: "AI Translator",
  headline: "Understand what you read.",
  lineWidths: [100, 78, 92, 61],
  highlightedLine: 1,
};

document.querySelector("#brand-name").textContent = creative.brand;
document.querySelector("#headline").textContent = creative.headline;

const readingLines = document.querySelector("#reading-lines");

creative.lineWidths.forEach((width, index) => {
  const line = document.createElement("span");
  line.className = "reading-line";
  line.style.width = `${width}%`;
  if (index === creative.highlightedLine) line.classList.add("highlight");
  readingLines.append(line);
});

document.documentElement.dataset.ready = "true";
