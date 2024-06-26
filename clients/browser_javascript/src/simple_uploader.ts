import axios, { AxiosRequestConfig } from "axios";
import camecaseKeys from "camelcase-keys";
import { PromisePool } from '@supercharge/promise-pool'

interface Progress {
  allSlice: number;
  finishedSlice: number;
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
  onCheckingSumProgress?: (progress: Progress) => void;
  requestOptions?: AxiosRequestConfig;
  prefix: string;
  concurrent: number;
  useV2?: boolean;
};

export default class SimpleUploader {
  meta?: FileMeta | null;
  metaKey: string;
  public options: SimpleUploaderOptions;
  halt: boolean;

  constructor(public file: File, options: Partial<SimpleUploaderOptions>) {
    this.options = Object.assign(
      {
        concurrent: 4,
        chunkSize: 1024 * 1024 * 10,
        endpoint: "/files",
        onProgress: undefined,
        prefix: "",
        useV2: true,
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
    this.meta = null;
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
          prefix: this.options.prefix
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

    let error: Error | null = null
    await PromisePool
      .for(slicesIds)
      .withConcurrency(this.options.concurrent)
      .handleError(async (e, _user, pool) => {
        if (e) {
          error = e
        }
        pool.stop();
      })
      .process(async (sliceId) => {
        if (this.halt) {
          this.halt = false;
          throw new UserCanceledUploading();
        }
        const slice = this.meta!.slices[sliceId];
        if (slice.status === 0) {
          const response = await this.uploadSlice(slice.sliceId);
          if (response.code == 206 || response.code == 200) {
            this.meta!.slices[slice.sliceId].status = 1;
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
      })

    if (error) {
      console.log("throwing error: ", error)
      throw error
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
      `${this.options.endpoint}/${this.meta!.fileId}/upload${this.options.useV2 ? '_v2' : ''}`,
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

  async checksum(): Promise<void> {
    const serverMeta = await axios.get<Response<FileMeta>>(
      `${this.options.endpoint}/${this.meta!.fileId}/meta`,
      this.options.requestOptions
    );
    let checksumIndex = 1;
    for (let sliceId in this.meta!.slices) {
      const slice = this.meta!.slices[sliceId];
      const sliceIdInt = parseInt(slice.sliceId);
      const file = this.file.slice(
        sliceIdInt * this.options.chunkSize,
        (sliceIdInt + 1) * this.options.chunkSize
      );
      const sha1 = await sha1File(file);

      if (serverMeta.data.data.slices[sliceId].sha1 !== sha1) {
        console.log(`slice ${sliceId} sha1 mismatch, reupload the slice`)
        let response = await this.uploadSlice(sliceId);
        if (response.code == 206 || response.code == 200) {
          console.log(`slice ${sliceId} reupload success`)
        } else {
          console.log(`slice ${sliceId} reupload failed`)
          // set the slice as not uploaded and then raise error
          this.meta!.slices[sliceId].status = 0;
          this.saveMeta();
          throw new Error("checksum failed")
        }
      }

      if (this.options.onCheckingSumProgress) {
        this.options.onCheckingSumProgress({
          allSlice: Object.keys(this.meta!.slices).length,
          finishedSlice: checksumIndex,
        });
        checksumIndex += 1;
      }
    }
  }
}
