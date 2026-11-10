const creative = {
  sourceTop: [100, 83],
  sourceBottom: [95, 71],
  resultLines: [100, 78, 90],
  primaryResultLine: 1,
};

function renderLines(selector, widths, className) {
  const container = document.querySelector(selector);
  widths.forEach((width, index) => {
    const line = document.createElement("span");
    line.className = className;
    line.style.width = `${width}%`;
    line.dataset.index = String(index);
    container.append(line);
  });
}

renderLines("#source-lines-top", creative.sourceTop, "copy-line");
renderLines("#source-lines-bottom", creative.sourceBottom, "copy-line");
renderLines("#result-lines", creative.resultLines, "result-line");

document
  .querySelectorAll("#result-lines .result-line")
  [creative.primaryResultLine].classList.add("primary");

document.fonts.ready.then(() => {
  document.documentElement.dataset.ready = "true";
});
