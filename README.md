# u9n
Size for the peak of the sum, not the sum of the peaks.

...and then size for less than that.

### Usage

Three subcommands, all reading the same csv.

| Command | What it answers |
| --- | --- |
| `u9n size` | How much vCPU the estate needs, sized three ways. |
| `u9n profile` | What the estate draws each hour of the day. |
| `u9n peak` | How much of the sum of the peaks is never drawn at once. |

#### size

```sh
u9n size --config _examples/quant-fund.csv --headroom 20 --growth 15 --years 3 --profile
```

| Flag | Default | Description |
| --- | --- | --- |
| `--config` | | Path to the csv file containing cluster vcpu data. Required. |
| `--headroom` | `20` | Percentage of spare capacity to add on top of peak utilisation. |
| `--growth` | `0` | Expected annual growth in vcpu demand, as a percentage. |
| `--years` | `1` | Number of years to compound growth over. |
| `--profile` | `false` | Print the hourly vcpu profile of the consolidated estate. |
| `--format` | `table` | Output format, one of `table`, `json` or `graph`. |

<p align="center">
  <img src="./assets/images/size.png" alt="size" width="400">
</p>

#### profile

```sh
u9n profile --config _examples/quant-fund.csv --base --peak
```

| Flag | Default | Description |
| --- | --- | --- |
| `--config` | | Path to the csv file containing cluster vcpu data. Required. |
| `--base` | `false` | Show the vcpu the estate holds around the clock. |
| `--peak` | `false` | Show the vcpu the peak windows add on top of the base. |

<p align="center">
  <img src="./assets/images/profile.png" alt="profile" width="400">
</p>

#### peak

```sh
u9n peak --config _examples/quant-fund.csv
```

| Flag | Default | Description |
| --- | --- | --- |
| `--config` | | Path to the csv file containing cluster vcpu data. Required. |

`size --profile` and the `profile` command are different things, which the shared word doesn't help with. The flag prints the consolidated estate's hourly figures as rows under the table. The command draws today's demand as a chart, split into the part that never goes away and the part the peak windows add.

<p align="center">
  <img src="./assets/images/peak.png" alt="peak" width="400">
</p>

### Config

A csv file with one row per cluster. Column order doesn't matter.

| Column | Required | Description |
| --- | --- | --- |
| `name` | yes | Cluster name. Must be unique. |
| `provisioned_vcpu` | yes | vCPU the cluster has today. |
| `peak_vcpu` | yes | vCPU the cluster uses at peak. |
| `base_vcpu` | no | vCPU the cluster uses outside its peak window. Defaults to `0`. |
| `peak_from` | with `peak_to` | Hour the peak window opens, `0`-`23`. |
| `peak_to` | with `peak_from` | Hour the peak window closes, `0`-`24`. |

```csv
name,provisioned_vcpu,peak_vcpu,base_vcpu,peak_from,peak_to
crm-app,24,18,4,9,17
batch-etl,16,11,3,22,3
monitoring,6,6,6,0,0
```

Peak windows are half-open, so `9` to `17` covers 09:00 until 16:59. A window may wrap midnight (`22` to `3` covers 22:00 until 02:59), and a window whose bounds are equal covers the whole day. Omit `peak_from` and `peak_to` entirely and every cluster is treated as permanently at peak.

All hours are assumed to be in the same timezone. Normalise to UTC before writing the config if the estate spans regions.

### Output

Example with zero growth.

```sh
u9n size --config _examples/quant-fund.csv --headroom 40
```

```text
Clusters:          14
Provisioned vCPU:  18576
Headroom:          40%
Peak diversity:    45.08%

               Current    Worst case     Hour aware
Required vCPU  18576      13032          7157
Peak vCPU      9308       9308           5112
Utilisation    50.11%     71.42%         71.43%
Peak hours     -          -              20:00, 21:00, 22:00
Saved vCPU     0 (0.00%)  5544 (29.84%)  11419 (61.47%)
```

Example with 15% growth every year for 3 years.

```sh
u9n size --config _examples/quant-fund.csv --headroom 40 --growth 15 --years 3
```

```text
Clusters:          14
Provisioned vCPU:  18576
Headroom:          40%
Growth:            15% a year over 3 years (52.09% in total)
Peak diversity:    45.08%

               Current    Worst case      Hour aware
Required vCPU  18576      19819           10885
Peak vCPU      9308       9308            5112
Utilisation    50.11%     46.97%          46.96%
Peak hours     -          -               20:00, 21:00, 22:00
Saved vCPU     0 (0.00%)  -1243 (-6.69%)  7691 (41.40%)
```

**Current** is the status quo. It's sized by what's already racked rather than by demand, so headroom and growth don't move it. It's the baseline the savings are measured against, which is why it always saves nothing.

**Worst case** is the sum of every cluster's peak. It assumes they all peak at the same instant, whenever that is, so it has no peak hour.

**Hour aware** walks the 24 hour clock, adds up what every cluster draws in each hour, and takes the busiest. A day often holds its peak for a stretch rather than a single hour, so every hour tied for the busiest is listed. Without peak windows in the config every hour is identical, so the column repeats the worst case and the peak hours read `all day`.

**Peak diversity** is the share of the worst case total avoided by peaks falling in different hours. It's the consolidation win.

Required vCPU is the peak figure, grown by `--growth` compounded over `--years`, plus `--headroom`, rounded up.

Peak vCPU is always a peak and never an average, but which peak depends on the column. Current and worst case both add up every cluster's own peak. Hour aware takes the busiest single hour of the overlaid windows, which is why it's lower. Both are today's demand, so growth shows up in the required row only.

Utilisation is peak demand against the capacity that sizing stands up, not against today's estate. A consolidated sizing runs hotter than the estate it replaces, and that's the point - dividing by hardware you'd no longer own would hide it. Headroom is what holds the figure below 100%.

Saved vCPU goes negative when a sizing asks for more than is already racked, as the worst case does in the growth example above. That isn't an error, it's the answer: at that headroom and that much growth, consolidating on the assumption every cluster peaks together would cost capacity rather than release it. Only the hour aware column still saves, and only because the peaks are staggered.

### Reading the result

Treat the two consolidated sizings as bounds, not as alternatives:

- **Worst case is the upper bound.** It assumes perfect correlation between clusters.
- **Hour aware is the lower bound.** It assumes the peak windows are exact and that jobs never overrun.

Peak windows are human estimates. A batch job that slips an hour collides with a window the model says it misses, so real demand sits between the two. Size above the hour aware figure unless the windows are backed by measurement.

Some things the tool deliberately doesn't model, each of which can move the answer more than headroom does:

- **Physical cores.** Output is vCPU. The core count to buy depends on the hypervisor's overcommit ratio.
- **Memory.** RAM usually binds before CPU in a consolidation.
- **The largest single cluster.** A VM can't span hosts, so no host can be smaller than the biggest cluster's peak.
- **Resilience.** One consolidated estate is one failure domain, with no N+1 reserve.
- **The calendar.** One representative day only, so no day-of-week, month-end or quarter-end peaks.
- **Hypervisor overhead.** The host OS and its storage and network services take their cut before any guest runs.

### Graph

`size --format graph` opens a terminal chart of what each sizing provisions across the day. Press `q` to quit.

<p align="center">
  <img src="./assets/images/size.png" alt="quant-fund" width="400">
</p>

There's one line per sizing: current (blue), worst case (green) and hour aware (purple), each with a translucent fill under it. All three are flat across the day, because every sizing racks one capacity and holds it, the hour aware one included - it's sized by the busiest hour rather than by every cluster's peak at once, but the hardware still sits there the other 23 hours. Each line sits at that sizing's required vCPU from the table.

What moves is the grey line along the bottom. That's utilisation, the demand the estate has to serve that hour, and the band between it and a sizing's line is capacity that sizing provisions but doesn't use. Utilisation is one curve rather than three, because the workload doesn't change with the way it's sized, so what separates the sizings is how much fill sits on top. The legend keys the grey separately from the three fills.

It's a curve rather than a staircase: a monotone cubic runs through the 24 hourly readings, wrapping from hour 23 back into hour 0 so the line doesn't flatten off against either edge. Monotone rather than a plain spline because a spline overshoots, and a demand curve bulging above the busiest hour it was drawn from would claim load the estate never sees.

The fills are translucent so the lines still read where they cross: the lower of two lines sharing a row keeps the row, and a fill never hides a line drawn under it. Every line takes a whole row, however little of it the value reaches into, so no line reads lighter than another for landing where it happens to land.

Without peak windows the hour aware sizing is the worst case, so only two lines are drawn rather than the same one twice.

The y axis is numbered at each line, in the colour of what it belongs to, plus a grey one for the busiest hour of the utilisation curve. Short terminals leave few rows to label, so where two of those fall in the same row the taller one takes the number.

The chart needs one column an hour plus the axis gutter, so around 30 columns. Below that it says so rather than wrapping the lines out of line with their hours. Above it, the hours widen to five columns each.

### Profile

`u9n profile` charts what the estate actually draws each hour, 00:00 to 23:00, as a stacked bar a day wide. Unlike the graph it isn't a sizing, so it carries no headroom and no growth, and it prints and exits rather than opening a terminal app.

Each bar is one hour, split in two. **Base** is the vCPU every cluster holds whether or not it's working, which is the same in all 24 hours and so draws as a flat floor. **Peak** is what the clusters inside their peak window add on top, which is the part that moves. Stacked, the two are that hour's whole draw, so the tallest bar is the hour aware peak vCPU from the table.

`--base` and `--peak` pick the bands. Dropping one rescales the chart rather than leaving a gap, so `--peak` on its own fills the height with the uplift instead of stranding it above a floor that isn't drawn. The axis is numbered at the tallest bar and at the top of the base band, since those are the two figures worth reading off it.

### Peak

`u9n peak` charts the two numbers the estate can be sized for, against each other. The sum of the peaks assumes every cluster peaks at the same instant. The peak of the sum is the busiest hour of the day they actually share. Like `profile` it draws demand rather than a sizing, so it carries no headroom and no growth: both numbers scale by the same factor under either, which moves them without moving the gap.

<p align="center">
  <img src="./assets/images/peak.png" alt="peak" width="400">
</p>

The top row is the sum of the peaks, drawn whole in the worst case colour. The two under it are that same bar taken apart: the solid part is what the estate is serving at its busiest hour, and the shaded part is capacity bought for peaks that never land in the same hour, which is Avoided, the peak diversity from the table.

Under it, every cluster is drawn as its own peak, biggest first and scaled against the biggest, with the part it contributes at the peak hour filled in and the rest shaded. A row that's almost all shade is a cluster the consolidated estate never pays for twice; a row that's solid is one that's at peak when everything else is. The solid parts add up to the peak of the sum and the whole bars to the sum of the peaks, so the rows account for the gap above them rather than restating it.

Clusters smaller than a fortieth of the biggest one are rounded up to a cell per band, so a sliver reads as a sliver rather than vanishing. Where the day holds its peak for more than one hour, the rows are read off the first of them, and a footnote says which hours those are, since they draw the same total from a different mix of clusters.

Last is the same comparison read across the whole day rather than in the one hour the bars were drawn from, since the busiest hour is the hour either sizing comes closest to being justified in. Each row is what the estate draws at one end of the day, against what both racks hold while it does: in the quant fund the peak of the sum is 45.08% idle at 20:00 but 89.79% idle at 06:00, and 69.80% idle averaged over the 24 hours. A run of hours too long to list is counted instead.

### JSON

`size --format json` emits the same figures plus per-cluster detail, for piping into other tools. The hourly profile is always included, so `--profile` affects table output only.

```sh
u9n size --config _examples/quant-fund.csv --format json | jq '.scenarios[] | {key, required_vcpu}'
```

```json
{
  "clusters": 14,
  "provisioned_vcpu": 18576,
  "headroom_percent": 20,
  "growth_percent": 0,
  "years": 1,
  "growth_factor": 1,
  "windowed": true,
  "peak_diversity_percent": 45.07950150408251,
  "scenarios": [
    {
      "key": "hour_aware",
      "label": "Hour aware",
      "peak_vcpu": 5112,
      "utilisation_percent": 83.32518337408314,
      "peak_hours": [20, 21, 22],
      "required_vcpu": 6135,
      "saved_vcpu": 12441,
      "saved_percent": 66.97351421188631
    }
  ],
  "hourly": [{ "hour": 0, "vcpu": 2116 }],
  "cluster_detail": [
    {
      "name": "market-data-feed",
      "provisioned_vcpu": 512,
      "peak_vcpu": 240,
      "base_vcpu": 160,
      "peak_from": 7,
      "peak_to": 17,
      "peak_hour_contribution_vcpu": 160
    }
  ]
}
```

`peak_hours` is null for sizings that aren't hour aware, and `peak_from`, `peak_to` and `peak_diversity_percent` are null when the config carries no peak windows. `peak_hour_contribution_vcpu` is what each cluster draws at the first of the peak hours, which is how you find the clusters driving the number. The peak hours all draw the same total, but not from the same mix of clusters.

### Examples

u9n comes with 10 examples with varying workloads and peaks. We'll explore just one (quant-fund).

A trading day that barely registers compared to a nightly batch chain that runs the estate flat out.

Profile the day

```sh
u9n profile --config _examples/quant-fund.csv
```

<p align="center">
  <img src="./assets/images/profile.png" alt="profile" width="400">
</p>

Run with 50% headroom (tabular output)

```sh
u9n size --config _examples/quant-fund.csv --headroom 50
```

Output

```
Clusters:          14
Provisioned vCPU:  18576
Headroom:          50%
Peak diversity:    45.08%

               Current    Worst case     Hour aware
Required vCPU  18576      13962          7668
Peak vCPU      9308       9308           5112
Utilisation    50.11%     66.67%         66.67%
Peak hours     -          -              20:00, 21:00, 22:00
Saved vCPU     0 (0.00%)  4614 (24.84%)  10908 (58.72%)
```

The batch stages are staggered rather than concurrent, which is where the 45% peak diversity comes from, and it's also the assumption most worth testing. Run the same config with the stages overlapping and the hour aware figure converges on the worst case.

Run with 50% headroom (graph output)

<p align="center">
  <img src="./assets/images/size.png" alt="quant-fund" width="400">
</p>

