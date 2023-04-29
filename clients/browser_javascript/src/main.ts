import SimpleUploader from "./simple_uploader";

window.addEventListener("load", () => {
  const file = document.querySelector("input[type=file]") as HTMLInputElement;
  const progressSpan = document.getElementById("progress") as HTMLSpanElement;

  file.onchange = async (e) => {
    const target = e.target as HTMLInputElement;
    if (!target.files || target.files.length <= 0) {
      return;
    }
    const uploader = new SimpleUploader(target.files[0], {
      onProgress: (progress) => {
        progressSpan.innerText = Math.ceil(progress.finishedSlice / progress.allSlice * 100).toString();
      },
    });
    await uploader.upload();
    uploader.clearMeta();
  };
});
