import { IconArrowUp, IconPhotoPlus, IconX } from "@tabler/icons-react"
import { useState } from "react"
import type { KeyboardEvent } from "react"
import { useTranslation } from "react-i18next"
import TextareaAutosize from "react-textarea-autosize"

import { ContextUsageRing } from "@/components/chat/context-usage-ring"
import { Button } from "@/components/ui/button"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"
import { useVoice } from "@/hooks/use-voice"
import { VoiceRecorder } from "./voice-recorder"
import type { ChatAttachment, ContextUsage } from "@/store/chat"

export type ChatInputDisabledReason =
  | "gatewayUnknown"
  | "gatewayStarting"
  | "gatewayRestarting"
  | "gatewayStopping"
  | "gatewayStopped"
  | "gatewayError"
  | "websocketConnecting"
  | "websocketDisconnected"
  | "websocketError"
  | "noDefaultModel"

interface ChatComposerProps {
  input: string
  attachments: ChatAttachment[]
  onInputChange: (value: string) => void
  onAddImages: () => void
  onRemoveAttachment: (index: number) => void
  onSend: () => void
  onContextDetail?: () => void
  inputDisabledReason: ChatInputDisabledReason | null
  canSend: boolean
  contextUsage?: ContextUsage
}

export function ChatComposer({
  input,
  attachments,
  onInputChange,
  onAddImages,
  onRemoveAttachment,
  onSend,
  onContextDetail,
  inputDisabledReason,
  canSend,
  contextUsage,
}: ChatComposerProps) {
  const { t } = useTranslation()
  const { outputEnabled, asrAvailable, ttsAvailable, streamingAvailable, toggleOutput } = useVoice()
  const [showRecorder, setShowRecorder] = useState(false)
  const canInput = inputDisabledReason === null
  const disabledMessage =
    inputDisabledReason === null
      ? null
      : t(`chat.disabledPlaceholder.${inputDisabledReason}`)
  const placeholder = disabledMessage ?? t("chat.placeholder")

  const handleSend = () => {
    if (showRecorder && streamingAvailable) {
      setShowRecorder(false)
      onSend()
      return
    }
    onSend()
  }

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.nativeEvent.isComposing) return
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }

  return (
    <div className="before:bg-background pointer-events-none relative z-10 -mt-[24px] shrink-0 overflow-y-auto px-4 pb-[calc(1rem+env(safe-area-inset-bottom))] [scrollbar-gutter:stable] before:pointer-events-none before:absolute before:inset-x-0 before:top-[24px] before:bottom-0 before:content-[''] md:px-8 md:pb-8 lg:px-24 xl:px-48">
      <div className="bg-card border-border/60 pointer-events-auto relative mx-auto flex max-w-[1000px] flex-col rounded-2xl border p-3 shadow-sm">
        {attachments.length > 0 && (
          <div className="mb-3 flex flex-wrap gap-2 px-2">
            {attachments.map((attachment, index) => (
              <div
                key={`${attachment.url}-${index}`}
                className="bg-background relative h-20 w-20 overflow-hidden rounded-xl border"
              >
                <img
                  src={attachment.url}
                  alt={attachment.filename || t("chat.uploadedImage")}
                  className="h-full w-full object-cover"
                />
                <button
                  type="button"
                  onClick={() => onRemoveAttachment(index)}
                  className="bg-background/85 text-foreground absolute top-1 right-1 inline-flex h-6 w-6 items-center justify-center rounded-full border shadow-sm transition hover:bg-white"
                  aria-label={t("chat.removeImage")}
                  title={t("chat.removeImage")}
                >
                  <IconX className="h-3.5 w-3.5" />
                </button>
              </div>
            ))}
          </div>
        )}

        {showRecorder && streamingAvailable && (
          <VoiceRecorder
            onInterimText={(text, _definite) => onInputChange(text)}
            onTranscribed={(text) => {
              onInputChange(text)
              setShowRecorder(false)
            }}
            onError={(err) => {
              console.warn("ASR error:", err)
              setShowRecorder(false)
            }}
          />
        )}

        <TextareaAutosize
          value={input}
          onChange={(e) => onInputChange(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={placeholder}
          disabled={!canInput}
          readOnly={showRecorder && streamingAvailable}
          title={disabledMessage || undefined}
          className={cn(
            "placeholder:text-muted-foreground/50 max-h-[200px] min-h-[64px] resize-none border-0 bg-transparent px-2 py-1 text-[15px] shadow-none transition-colors focus-visible:ring-0 focus-visible:outline-none dark:bg-transparent",
            !canInput && "cursor-not-allowed",
          )}
          minRows={1}
          maxRows={8}
        />

        <div className="mt-2 flex items-center justify-between px-1">
          <div className="flex items-center gap-1">
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="text-muted-foreground hover:text-foreground h-8 w-8 rounded-full"
              onClick={onAddImages}
              disabled={!canInput}
              aria-label={t("chat.attachImage")}
              title={t("chat.attachImage")}
            >
              <IconPhotoPlus className="size-4" />
            </Button>
            {streamingAvailable ? (
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className={`text-muted-foreground hover:text-foreground h-8 w-8 rounded-full ${showRecorder ? "bg-violet-100 text-violet-600" : ""}`}
                onClick={() => setShowRecorder((v) => !v)}
                disabled={!canInput}
                title={showRecorder ? "停止录音" : "语音输入"}
              >
                <span className="text-base">{showRecorder ? "🎙️" : "🎤"}</span>
              </Button>
            ) : null}
            {ttsAvailable && (
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className={`text-muted-foreground hover:text-foreground h-8 w-8 rounded-full ${outputEnabled ? "bg-violet-100 text-violet-600" : ""}`}
                onClick={toggleOutput}
                disabled={!canInput}
                title={outputEnabled ? "关闭语音朗读" : "开启语音朗读"}
                aria-label={outputEnabled ? "关闭语音朗读" : "开启语音朗读"}
              >
                <span className="text-base">{outputEnabled ? "🔊" : "🔈"}</span>
              </Button>
            )}
          </div>

          <div className="flex items-center gap-1.5">
            {contextUsage && (
              <ContextUsageRing
                usage={contextUsage}
                onDetailClick={onContextDetail}
              />
            )}
            {canInput ? (
              <Tooltip delayDuration={700}>
                <TooltipTrigger asChild>
                  <span tabIndex={!canSend ? 0 : undefined}>
                    <Button
                      type="button"
                      size="icon"
                      className="size-8 rounded-full bg-violet-500 text-white transition-transform hover:bg-violet-600 active:scale-95"
                      onClick={handleSend}
                      disabled={!canSend}
                      aria-label={t("chat.sendMessage")}
                    >
                      <IconArrowUp className="size-4" />
                    </Button>
                  </span>
                </TooltipTrigger>
                <TooltipContent
                  className="border-border/70 bg-muted text-foreground border text-center whitespace-pre-line shadow-lg shadow-black/10 dark:shadow-black/30"
                  arrowClassName="bg-muted fill-muted"
                >
                  {t("chat.sendHint")}
                </TooltipContent>
              </Tooltip>
            ) : null}
          </div>
        </div>
      </div>
    </div>
  )
}
