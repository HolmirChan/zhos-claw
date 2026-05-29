import { useRef, useState } from "react";

interface AudioPlayerProps {
  audioUrl: string;
}

export function AudioPlayer({ audioUrl }: AudioPlayerProps) {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const [playing, setPlaying] = useState(false);

  return (
    <div className="audio-player mt-2 flex items-center gap-2">
      <button
        type="button"
        onClick={() => {
          if (!audioRef.current) return;
          if (playing) {
            audioRef.current.pause();
          } else {
            audioRef.current.play();
          }
          setPlaying(!playing);
        }}
        className="text-muted-foreground hover:text-foreground inline-flex h-7 w-7 items-center justify-center rounded-full transition-colors"
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
    </div>
  );
}
