export function FlowDashes({ delay = false }: { delay?: boolean }): React.JSX.Element {
  return (
    <div className="relative h-0.5 flex-1 overflow-hidden bg-[repeating-linear-gradient(90deg,rgb(var(--color-primary)/0.45)_0_6px,transparent_6px_12px)]">
      <span
        className="absolute -left-16 -top-0.5 h-1.5 w-11 rounded-full bg-gradient-to-r from-transparent to-primary"
        style={{ animation: `bridgeFlow 1.9s linear ${delay ? '0.5s' : '0s'} infinite` }}
      />
    </div>
  )
}
