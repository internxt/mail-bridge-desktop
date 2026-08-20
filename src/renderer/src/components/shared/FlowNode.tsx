export const FlowNode = ({
  icon,
  title,
  subtitle,
  tone
}: {
  icon: React.ReactNode
  title: string
  subtitle: string
  tone: 'surface' | 'accent'
}): React.JSX.Element => {
  return (
    <div className="flex flex-none items-center gap-2">
      <div
        className={`grid h-[34px] w-[34px] place-items-center rounded-[9px] ${
          tone === 'accent'
            ? 'bg-primary text-white'
            : 'border border-primary/20 bg-surface text-primary'
        }`}
      >
        {icon}
      </div>
      <div>
        <div className="text-[12.5px] font-bold text-gray-100">{title}</div>
        <div className={`text-[11.5px] ${tone === 'accent' ? 'text-primary' : 'text-gray-60'}`}>
          {subtitle}
        </div>
      </div>
    </div>
  )
}
