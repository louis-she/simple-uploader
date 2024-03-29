import hashlib
import io
import json
import os
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Any, Callable, Dict, List, Optional, Union

import requests
from dacite import from_dict


@dataclass
class CheckResult:
    success_count: int
    failed_count: int
    failed_slices_id: List[str]


@dataclass
class Progress:
    all_slice: int
    finished_slice: int


@dataclass
class Slice:
    slice_id: str
    status: int
    sha1: str


@dataclass
class FileMeta:
    file_id: str
    file_name: str
    file_type: str
    file_size: int
    chunk_size: int
    prefix: str
    created_at: int
    status: int
    slices: Dict[str, Slice]


@dataclass
class Options:
    chunk_size: Optional[int] = 1024 * 1024 * 10
    endpoint: Optional[str] = "http://127.0.0.1:8080/files"
    on_progress: Optional[Callable[[Progress], None]] = None
    prefix: Optional[str] = ""
    headers: Optional[Dict[str, str]] = None
    use_v2: Optional[bool] = True


@dataclass
class Response:
    code: int
    message: str
    data: Optional[Any] = None


class SimpleUploader:
    file: Path
    options: Options
    meta: FileMeta
    meta_key: str
    meta_file: Path

    def __init__(
        self, file: Union[Path, str], options: Optional[Union[dict, Options]] = {}
    ):
        self.file = Path(file)
        self.fh = self.file.open("rb")
        self.file_size = os.path.getsize(self.file)
        if isinstance(options, dict):
            self.options = Options(**options)
        self.meta = None
        self.meta_key = f"file_meta_{self.file.name}_{self.file_size}"
        self.meta_file = (
            Path.home() / ".cache" / "simple_uploader" / (self.meta_key + ".json")
        )
        self.meta_file.parent.mkdir(parents=True, exist_ok=True)
        self.meta_file.touch(exist_ok=True)
        self.load_meta()

    def clear_meta(self):
        self.meta = None
        self.meta_file.unlink()

    def load_meta(self):
        try:
            self.meta = from_dict(FileMeta, json.loads(self.meta_file.read_text()))
        except json.decoder.JSONDecodeError:
            pass

    def save_meta(self):
        self.meta_file.write_text(json.dumps(asdict(self.meta)))

    def upload(self):
        if not self.meta:
            response = requests.post(
                self.options.endpoint,
                json={
                    "file_name": self.file.name,
                    "file_type": self.file.suffix,
                    "file_size": self.file_size,
                    "chunk_size": self.options.chunk_size,
                    "prefix": self.options.prefix,
                },
                headers={
                    **(self.options.headers if self.options.headers is not None else {})
                },
            )
            if response.status_code != 200:
                raise Exception(response.text)
            self.meta = from_dict(FileMeta, response.json()["data"])
            self.save_meta()

        for slice_id, slice in self.meta.slices.items():
            if slice.status == 0:
                response = self._upload_slice(slice_id)
                if response.code == 206 or response.code == 200:
                    self.meta.slices[slice_id].status = 1
                    self.save_meta()
                if self.options.on_progress:
                    self.options.on_progress(
                        {
                            "all_slice": len(self.meta.slices),
                            "finished_slice": len(
                                list(
                                    filter(
                                        lambda s: s.status == 1,
                                        self.meta.slices.values(),
                                    )
                                )
                            ),
                        }
                    )

    def _upload_slice(self, slice_id: str) -> Response:
        self.fh.seek(int(slice_id) * self.options.chunk_size)
        bytes = self.fh.read(self.options.chunk_size)
        form_data = {
            "slice_id": slice_id,
            "file_id": self.meta.file_id,
            "file_name": self.meta.file_name,
            "file_type": self.meta.file_type,
            "file_size": self.meta.file_size,
            "chunk_size": self.options.chunk_size,
        }
        response = requests.post(
            f"{self.options.endpoint}/{self.meta.file_id}/upload{'_v2' if self.options.use_v2 else ''}",
            data=form_data,
            files={
                "file": io.BytesIO(bytes),
            },
            headers={
                **(self.options.headers if self.options.headers is not None else {})
            },
        )

        if response.status_code >= 400:
            raise Exception(response.text)

        return from_dict(Response, response.json())

    def _sha1(self, blob: bytes) -> str:
        sha1 = hashlib.sha1()
        sha1.update(blob)
        return sha1.hexdigest()

    def checksum(self) -> CheckResult:
        response = requests.get(
            f"{self.options.endpoint}/{self.meta.file_id}/meta",
            headers={
                **(self.options.headers if self.options.headers is not None else {})
            },
        )
        server_meta = from_dict(FileMeta, response.json()["data"])
        check_result = CheckResult(0, 0, [])
        for slice_id, slice in server_meta.slices.items():
            self.fh.seek(int(slice_id) * self.options.chunk_size)
            blob = self.fh.read(self.options.chunk_size)
            sha1 = self._sha1(blob)
            if slice.sha1 == sha1:
                check_result.success_count += 1
            else:
                check_result.failed_count += 1
                check_result.failed_slices_id.append(slice_id)
        return check_result
