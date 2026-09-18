# Recorded compaction demo

Open [index.html](index.html) in a browser. It runs locally without a server or network requests.
Choose one of three tasks. Use the chapters, timeline, or play button to inspect each step.

The demo shows recorded inputs, live Jev scores, selected excerpts, actual Codex patches, and held-out test results.
It visualizes removal from active context. The archive retains the original content.
The displayed source lines are a condensed view; JSON evidence contains the complete selected history.

**This is a trace replay with normalized timing.** It is not a recording of live agent execution.
The 32-second animation does not represent measured execution time.
The compaction metric shows the separately recorded duration.

- [GIF](assets/compact-engine.gif), suitable for a repository README.
- [MP4](assets/compact-engine.mp4), suitable for sharing.
- [Poster](assets/poster.png).
- [Pilot results and limitations](../docs/results/agent-pilot.md).

## Rebuild the media

The renderer uses Node.js, Playwright 1.62.1, local Google Chrome, and FFmpeg.
Install optional rendering dependencies under the ignored reports directory:

```sh
npm install --prefix reports/demo-tools --no-save playwright@1.62.1
NODE_PATH="$PWD/reports/demo-tools/node_modules" node demo/capture.mjs reports/demo-frames 10
mkdir -p demo/assets
ffmpeg -y -framerate 10 -i reports/demo-frames/%04d.png \
  -c:v libx264 -crf 20 -pix_fmt yuv420p -movflags +faststart demo/assets/compact-engine.mp4
ffmpeg -y -i demo/assets/compact-engine.mp4 \
  -filter_complex 'fps=8,scale=960:-1:flags=lanczos,split[a][b];[a]palettegen=stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=3' \
  -loop 0 demo/assets/compact-engine.gif
cp reports/demo-frames/0180.png demo/assets/poster.png
```

The video shows the tenant cache task. The interactive version includes all three tasks.
The renderer captures the same HTML and recorded data used by the interactive version.
It also writes a result screenshot for each task.
