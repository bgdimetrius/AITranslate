const form = document.querySelector('#download-form');
const keyInput = document.querySelector('#access-key');
const button = form.querySelector('button');
const status = document.querySelector('#status');

form.addEventListener('submit', async (event) => {
  event.preventDefault();
  button.disabled = true;
  status.className = '';
  status.textContent = 'Checking access key…';

  try {
    const response = await fetch('/latest/ai-translate-extension.zip', {
      headers: { Authorization: `Bearer ${keyInput.value}` },
      cache: 'no-store',
    });

    if (!response.ok) {
      throw new Error('The access key is not valid.');
    }

    const packageBlob = await response.blob();
    const packageUrl = URL.createObjectURL(packageBlob);
    const link = document.createElement('a');
    link.href = packageUrl;
    link.download = 'ai-translate-extension.zip';
    document.body.append(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(packageUrl);
    keyInput.value = '';
    status.textContent = 'Download started.';
  } catch (error) {
    status.className = 'error';
    status.textContent = error instanceof Error ? error.message : 'Download failed.';
  } finally {
    button.disabled = false;
  }
});
