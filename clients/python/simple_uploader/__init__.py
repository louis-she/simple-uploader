import requests
import json
from dataclasses import dataclass
from typing import Optional, TypedDict, Callable, Union


@dataclass
class Progress:
    all_slice: int
    finished_slice: int


@dataclass
class Slice:
    slice_id: str
    status: int
    sha1: str


class FileMeta:
    file_id: str
    file_name: str
    file_type: str
    file_size: int
    chunk_size: int
    prefix: str
    created_at: int
    status: int
    slices: dict[str, Slice]


@dataclass
class Options:
    chunk_size: Optional[int] = 1024 * 1024 * 10
    endpoint: Optional[str] = "/files"
    on_progress: Optional[Callable[[Progress]]] = None


class SimpleUploader:

    file: file
    options: Options
    meta: FileMeta

    def __init__(self, file, options: Union[dict, Options]):
        self.file = file
        if isinstance(options, dict):
            self.options = Options(**options)
