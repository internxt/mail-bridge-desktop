export const TabButton = ({
  active,
  onClick,
  icon,
  label,
  note
}: {
  active: boolean
  onClick: () => void
  icon: React.ReactNode
  label: string
  note: string
}): React.JSX.Element => {
  return (
    <button
      onClick={onClick}
      className={`flex flex-1 items-center justify-center gap-2 rounded-lg px-3 py-2 text-[12.5px] font-bold tracking-[0.04em] ${
        active
          ? 'bg-surface text-primary shadow-sm'
          : 'text-gray-60 hover:bg-gray-5 hover:text-gray-100'
      }`}
    >
      <span className={active ? 'text-primary' : 'text-gray-40'}>{icon}</span>
      {label}
      <span className="text-[11px] font-medium text-gray-50">{note}</span>
    </button>
  )
}
