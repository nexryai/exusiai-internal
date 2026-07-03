# Usage Guide for Next.js Backend Agents

This document is intended to be read by coding agents that implement a Next.js backend or BFF using this microservice. Treat it as operational instructions and API contract documentation.

## Service Role

This service archives YouTube videos for personal use. A Next.js backend should call this service instead of running `yt-dlp`, `ffmpeg`, YouTube Data API, MongoDB writes, or S3-compatible uploads directly.

The service performs the following work:

- Fetches YouTube metadata through the official YouTube Data API.
- Creates a MongoDB archive job under the provided application `userId`.
- Starts `yt-dlp` as a child process.
- Remuxes the downloaded video into DASH using system `ffmpeg` with `-c copy`.
- Uploads DASH files to S3-compatible storage under `[database video id]/...`.
- Downloads the YouTube thumbnail, converts it to AVIF with `ffmpeg`, and uploads it to `[database video id]/thumbnail.avif`.
- Tracks job status in MongoDB.

## Base URL

Configure the Next.js backend with the internal base URL of this service.

Example:

```env
EXUSIAI_INTERNAL_URL=http://localhost:8080
```

Do not expose this service directly to browsers unless an authentication layer is added.

## Authentication

This service currently has no built-in authentication or authorization. The Next.js backend must authenticate the end user and pass its own trusted application user ID as `userId`.

Never accept `userId` directly from an untrusted browser request. Derive it from the authenticated Next.js session.

## Endpoints

### Health Check

```http
GET /
```

Successful response:

```json
{
  "status": "I'm OK!"
}
```

Use this only for service readiness checks.

### Add Video to Queue

```http
POST /v1/queue/add
Content-Type: application/json
```

Request body:

```json
{
  "userId": "app-user-id",
  "url": "https://www.youtube.com/watch?v=dQw4w9WgXcQ"
}
```

Successful response:

```json
{
  "id": "4f8f0e4f4eb243dba61a4d64ab7f3f46",
  "status": "pending"
}
```

Important behavior:

- A `200` response means YouTube metadata was fetched, a MongoDB record was created with `status: "pending"`, and `yt-dlp` was successfully started.
- Downloading, DASH packaging, thumbnail conversion, and S3 upload continue asynchronously.
- Store the returned `id` in the Next.js application database if the frontend needs to track this archive job later.

Possible error responses:

```json
{
  "error": "userId and url are required"
}
```

```json
{
  "error": "youtube video id not found"
}
```

### Get Queue Status

```http
GET /v1/queue/{id}/status
```

Successful pending response:

```json
{
  "id": "4f8f0e4f4eb243dba61a4d64ab7f3f46",
  "status": "pending",
  "updatedAt": "2026-07-03T12:34:56Z"
}
```

Successful completed response:

```json
{
  "id": "4f8f0e4f4eb243dba61a4d64ab7f3f46",
  "status": "completed",
  "storage": {
    "url": "https://cdn.example.com/archive/4f8f0e4f4eb243dba61a4d64ab7f3f46/manifest.mpd",
    "bucket": "videos",
    "key": "4f8f0e4f4eb243dba61a4d64ab7f3f46/manifest.mpd",
    "contentType": "application/dash+xml",
    "sizeBytes": 1234,
    "uploadedAt": "2026-07-03T12:40:00Z"
  },
  "thumbnail": {
    "url": "https://cdn.example.com/archive/4f8f0e4f4eb243dba61a4d64ab7f3f46/thumbnail.avif",
    "bucket": "videos",
    "key": "4f8f0e4f4eb243dba61a4d64ab7f3f46/thumbnail.avif",
    "contentType": "image/avif",
    "sizeBytes": 5678,
    "uploadedAt": "2026-07-03T12:40:01Z"
  },
  "updatedAt": "2026-07-03T12:40:01Z"
}
```

Successful failed response:

```json
{
  "id": "4f8f0e4f4eb243dba61a4d64ab7f3f46",
  "status": "failed",
  "failure": {
    "message": "ffmpeg dash packaging failed: ...",
    "stage": "package",
    "failedAt": "2026-07-03T12:38:00Z"
  },
  "updatedAt": "2026-07-03T12:38:00Z"
}
```

Not found response:

```json
{
  "error": "video not found"
}
```

## Status Values

The Next.js backend should handle these status values:

- `pending`: The job is accepted. Downloading, DASH packaging, thumbnail conversion, or uploading may still be running.
- `completed`: The DASH manifest and AVIF thumbnail are available.
- `failed`: The job failed. Inspect `failure.stage` and `failure.message`.

The Go code also defines `processing`, but current API behavior keeps queued work as `pending` until it becomes `completed` or `failed`.

## Stored Object Layout

For a returned database video ID:

```text
[videoId]/manifest.mpd
[videoId]/init_$RepresentationID$.[original-extension]
[videoId]/chunk_$RepresentationID$_$Number$.[original-extension]
[videoId]/thumbnail.avif
```

The video is remuxed with `ffmpeg -c copy`; it is not re-encoded. The DASH chunk extension follows the downloaded media extension, such as `webm`, `mp4`, or `mkv`.

Use `storage.url` as the DASH manifest URL for playback. Use `thumbnail.url` as the poster image URL.

## Next.js Backend Example

Recommended server-side TypeScript types:

```ts
type QueueStatus = "pending" | "processing" | "completed" | "failed";

type StorageObject = {
  url?: string;
  bucket?: string;
  key?: string;
  contentType?: string;
  sizeBytes?: number;
  uploadedAt?: string;
};

type QueueAddResponse = {
  id: string;
  status: QueueStatus;
};

type QueueStatusResponse = {
  id: string;
  status: QueueStatus;
  storage?: StorageObject;
  thumbnail?: StorageObject;
  failure?: {
    message?: string;
    stage?: string;
    failedAt?: string;
  };
  updatedAt: string;
};
```

Example enqueue helper:

```ts
export async function enqueueYouTubeArchive(params: {
  userId: string;
  url: string;
}): Promise<QueueAddResponse> {
  const baseUrl = process.env.EXUSIAI_INTERNAL_URL;
  if (!baseUrl) throw new Error("EXUSIAI_INTERNAL_URL is not configured");

  const res = await fetch(`${baseUrl}/v1/queue/add`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(params),
    cache: "no-store",
  });

  const body = await res.json();
  if (!res.ok) {
    throw new Error(body.error ?? "Failed to enqueue video archive");
  }

  return body;
}
```

Example status helper:

```ts
export async function getYouTubeArchiveStatus(
  id: string,
): Promise<QueueStatusResponse> {
  const baseUrl = process.env.EXUSIAI_INTERNAL_URL;
  if (!baseUrl) throw new Error("EXUSIAI_INTERNAL_URL is not configured");

  const res = await fetch(`${baseUrl}/v1/queue/${encodeURIComponent(id)}/status`, {
    method: "GET",
    cache: "no-store",
  });

  const body = await res.json();
  if (!res.ok) {
    throw new Error(body.error ?? "Failed to get video archive status");
  }

  return body;
}
```

## Polling Guidance

After enqueueing, poll `GET /v1/queue/{id}/status` from the Next.js backend.

Recommended behavior:

- Poll every 3 to 10 seconds while `status` is `pending`.
- Stop polling when `status` is `completed` or `failed`.
- On `completed`, persist `storage.url` and `thumbnail.url` in the Next.js application database if useful.
- On `failed`, show a generic user-facing error and log `failure.stage` and `failure.message` server-side.

## Playback Guidance

The completed `storage.url` points to a DASH MPD manifest. Browser playback usually requires a DASH player such as `dash.js` or Shaka Player on the frontend.

The Next.js backend should not proxy the media chunks unless there is a product requirement to hide the object storage URLs. Prefer serving the manifest and chunks from the configured CDN or S3-compatible public endpoint.

## Error Handling Rules for Agents

When integrating from Next.js:

- Treat non-2xx responses as errors.
- Do not retry `POST /v1/queue/add` blindly; retries may create duplicate jobs.
- It is safe to retry `GET /v1/queue/{id}/status`.
- Log response bodies from failed service calls on the server side.
- Do not expose internal error details directly to end users.

## Service Runtime Requirements

The microservice must be configured with:

```env
MONGODB_URI=
YOUTUBE_API_KEY=
AWS_ACCESS_KEY_ID=
AWS_SECRET_ACCESS_KEY=
AWS_S3_BUCKET=
AWS_S3_REGION=
AWS_S3_ENDPOINT=
```

Optional:

```env
PORT=8080
MONGODB_DATABASE=exusiai_internal
DOWNLOAD_WORK_DIR=/tmp
FFMPEG_PATH=ffmpeg
PUBLIC_OBJECT_BASE_URL=
```

`PUBLIC_OBJECT_BASE_URL` should be set when object URLs should point to a CDN or public bucket URL instead of the raw S3-compatible endpoint.

