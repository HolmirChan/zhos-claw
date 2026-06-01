import { useRef, useState } from "react";

interface AudioPlayerProps {
  audioUrl?: string; // undefined = loading state → show spinner
}

export function AudioPlayer({ audioUrl }: AudioPlayerProps) {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const [playing, setPlaying] = useState(false);

  if (!audioUrl) {
    return (
      <span className="inline-flex h-6 w-6 items-center justify-center" title="正在生成语音...">
        <span className="block h-3 w-3 animate-spin rounded-full border-2 border-current border-t-transparent opacity-60" />
      </span>
    );
  }

  return (
    <span className="inline-flex items-center gap-0.5">
      <button
        type="button"
        onClick={() => {
          if (!audioRef.current) return;
          if (playing) { audioRef.current.pause(); }
          else { audioRef.current.play(); }
          setPlaying(!playing);
        }}
        className="inline-flex h-6 w-6 items-center justify-center rounded-full text-muted-foreground hover:text-foreground transition-colors"
        title={playing ? "暂停" : "播放"}
      >
        {playing ? "⏸" : "▶"}
      </button>
      <audio
        ref={audioRef}
        src={audioUrl}
        onEnded={() => setPlaying(false)}
        onPlay={() => setPlaying(true)}
        onPause={() => setPlaying(false)}
        autoPlay
      />
    </span>
  );
}
