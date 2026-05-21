# Changelog

All notable changes to this project will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/).

## [Unreleased]

### Added

- WebSocket upgrade endpoints for player (`/sessions/{id}/ws`) and observer (`/sessions/{id}/ws/observe`) connections
- HTTP session handlers: `POST /sessions`, `GET /sessions`, `GET /sessions/{id}`, `PATCH /sessions/{id}`, `POST /sessions/{id}/start`, `DELETE /sessions/{id}`
- Bearer token authentication on player WebSocket upgrades
- WebSocket message size limit (64 KB default, configurable via `WSReadLimit`)
- Nil-safe snapshot handling throughout the session layer
- Marshal-failure defense: sessions skip empty snapshots rather than sending nil frames to WebSocket clients
