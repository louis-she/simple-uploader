import axios, { AxiosRequestConfig } from "axios";
import camecaseKeys from "camelcase-keys";

interface Progress {
  allSlice: number;
  finishedSlice: number;
}

interface CheckResult {
  successCount: number;
  failedCount: number;
  failedSlicesId: string[];
}

interface Response<T> {
  code: number;
  message: string;
  data: T;
}

interface Slice {
  sliceId: string;
  status: number;
  sha1: string;
}

interface FileMeta {
  fileId: string;
  fileName: string;
  fileType: string;
  fileSize: number;
  chunkSize: number;
  prefix: string;
  createdAt: number;
  status: number;
  slices: { [key: string]: Slice };
}

export class UserCanceledUploading extends Error {
  constructor() {
    super("User Canceled Upload");
    this.name = "UserCanceledUploading"
  }
}

function sha1File(file: Blob): Promise<string> {
  return new Promise((resolve, _) => {
    const reader = new FileReader();
    reader.onload = async () => {
      const res = await crypto.subtle.digest(
        "SHA-1",
        reader.result as ArrayBuffer
      );
      const hashArray = Array.from(new Uint8Array(res));
      const hashHex = hashArray
        .map((b) => b.toString(16).padStart(2, "0"))
        .join("");
      return resolve(hashHex);
    };
    reader.readAsArrayBuffer(file);
  });
}

type SimpleUploaderOptions = {
  chunkSize: number;
  endpoint: string;
  onProgress?: (progress: Progress) => void;
  requestOptions?: AxiosRequestConfig;
};

export default class SimpleUploader {
  meta?: FileMeta | null;
  metaKey: string;
  public options: SimpleUploaderOptions;
  halt: boolean;

  constructor(public file: File, options: Partial<SimpleUploaderOptions>) {
    this.options = Object.assign(
      {
        chunkSize: 1024 * 1024 * 10,
        endpoint: "/files",
        onProgress: undefined,
      },
      options
    );
    this.meta = null;
    this.metaKey = `file_meta_${this.file.name}_${this.file.size}`;
    this.loadMeta();
    this.halt = false;
    if (this.meta) {
      console.log("meta data loaded from local storage", this.meta);
    }
  }

  clearMeta() {
    localStorage.removeItem(this.metaKey);
  }

  loadMeta() {
    const data = localStorage.getItem(this.metaKey);
    if (data) {
      try {
        this.meta = JSON.parse(data);
      } catch {
        console.log(`failed to parse meta data for file ${this.file.name}`);
      }
    }
  }

  cancel() {
    this.halt = true;
  }

  async upload() {
    if (!this.meta) {
      const response = await axios.post<Response<FileMeta>>(
        this.options.endpoint,
        {
          file_name: this.file.name,
          file_type: this.file.type,
          file_size: this.file.size,
          chunk_size: this.options.chunkSize,
        },
        this.options.requestOptions
      );
      if (response.status !== 200) {
        throw new Error(response.statusText);
      }
      this.meta = camecaseKeys(response.data.data, { deep: true });
      this.saveMeta();
    }

    const slicesIds = Object.keys(this.meta!.slices)

    async function* slicesIter() {
      for (let sliceId in slicesIds) {
        yield sliceId
      }
    }

    for await (let sliceId of slicesIter()) {
      if (this.halt) {
        this.halt = false;
        throw new UserCanceledUploading();  
      }
      const slice = this.meta!.slices[sliceId];
      if (slice.status === 0) {
        const response = await this.uploadSlice(slice.sliceId);
        if (response.code == 206 || response.code == 200) {
          this.meta.slices[slice.sliceId].status = 1;
          this.saveMeta();
        }
        if (this.options.onProgress) {
          this.options.onProgress({
            allSlice: Object.keys(this.meta!.slices).length,
            finishedSlice: Object.values(this.meta!.slices).filter(
              (s) => s.status === 1
            ).length,
          });
        }
      }
    }
  }

  async uploadSlice(sliceId: string): Promise<Response<string>> {
    const formData = new FormData();
    const sliceIdInt = parseInt(sliceId);
    formData.append(
      "file",
      this.file.slice(
        sliceIdInt * this.options.chunkSize,
        (sliceIdInt + 1) * this.options.chunkSize
      ),
      this.file.name
    );
    formData.append("slice_id", sliceId);
    formData.append("file_id", this.meta!.fileId);
    formData.append("file_name", this.meta!.fileName);
    formData.append("file_type", this.meta!.fileType);
    formData.append("file_size", this.meta!.fileSize.toString());
    formData.append("chunk_size", this.options.chunkSize.toString());

    const response = await axios.post<Response<string>>(
      `${this.options.endpoint}/${this.meta!.fileId}/upload`,
      formData,
      {
        ...this.options.requestOptions,
        headers: {
          ...(this.options.requestOptions &&
            this.options.requestOptions.headers),
          "Content-Type": "multipart/form-data",
        },
      }
    );

    if (response.status >= 400) {
      throw new Error(
        `failed to upload slice ${sliceId}, error: ${response.statusText}`
      );
    }

    return response.data;
  }

  saveMeta() {
    localStorage.setItem(this.metaKey, JSON.stringify(this.meta));
  }

  async checksum(): Promise<CheckResult> {
    const serverMeta = await axios.get<Response<FileMeta>>(
      `${this.options.endpoint}/${this.meta!.fileId}/meta`,
      this.options.requestOptions
    );
    let checkResult: CheckResult = {
      successCount: 0,
      failedCount: 0,
      failedSlicesId: [],
    };
    for (let sliceId in this.meta!.slices) {
      const slice = this.meta!.slices[sliceId];
      const sliceIdInt = parseInt(slice.sliceId);
      const file = this.file.slice(
        sliceIdInt * this.options.chunkSize,
        (sliceIdInt + 1) * this.options.chunkSize
      );
      const sha1 = await sha1File(file);
      if (serverMeta.data.data.slices[sliceId].sha1 === sha1) {
        checkResult.successCount += 1;
      } else {
        checkResult.failedCount += 1;
        checkResult.failedSlicesId.push(sliceId);
      }
    }
    return checkResult;
  }
}
