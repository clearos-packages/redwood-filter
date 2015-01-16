# Redwood Content Filter RPM spec
Name: redwood-filter
Version: 1.0
Release: 1%{dist}
Vendor: ClearFoundation
License: GPL
Group: System Environment/Daemons
Packager: ClearFoundation
Source: %{name}-%{version}.tar.gz
BuildRoot: /var/tmp/%{name}-%{version}
Summary: Redwood Content Filter
BuildRequires: golang git mercurial
Requires: initscripts /sbin/service
Requires(pre): /sbin/ldconfig, /usr/sbin/useradd, /usr/bin/getent
Requires(postun): /usr/sbin/userdel
Requires(post): /sbin/chkconfig
Requires(preun): /sbin/chkconfig

%description
Redwood Content Filter
Report bugs to: http://www.clearfoundation.com/docs/developer/bug_tracker/

# Build
%prep
mkdir -vp $(pwd)/go/{bin,pkg,src}

%setup -q
GOPATH=$(pwd)/go go get code.google.com/p/redwood-filter

%build
GOPATH=$(pwd)/go go install code.google.com/p/redwood-filter

# Install
%install
mkdir -vp %{buildroot}/%{_sbindir}
mkdir -vp %{buildroot}/%{_sysconfdir}
mkdir -vp %{buildroot}/%{_localstatedir}/log/redwood
install -D -m 755 go/bin/redwood-filter %{buildroot}/%{_sbindir}/redwood-filter
cp -vr go/src/code.google.com/p/redwood-filter/config %{buildroot}/%{_sysconfdir}/redwood

%if "0%{dist}" == "0.v6"
install -D -m 644 go/src/code.google.com/p/redwood-filter/startup/redhat %{buildroot}/%{_sysconfdir}/init.d/redwood-filter
#%else
#install -D -m 644 sysconf/redwood-filter.service %{buildroot}/lib/systemd/system/redwood-filter.service
#install -D -m 644 sysconf/redwood-filter.tmp %{buildroot}/%{_tmpfilesdir}/redwood-filter.conf
%endif

# Clean-up
%clean
[ "%{buildroot}" != / ] && rm -rf "%{buildroot}"

# Pre install
%pre
/usr/bin/getent passwd redwood 2>&1 >/dev/null ||\
    /usr/sbin/useradd -M -c "Redwood Filter" -r -d %{_sbindir}/redwood-filter -s /bin/false redwood 2> /dev/null || :

%preun
%if "0%{dist}" == "0.v6"
if [ "$1" = 0 ]; then
    /sbin/chkconfig --del redwood-filter
fi
#%else
#if [ "$1" = 0 ]; then
#    /sbin/chkconfig --del redwood-filter
#    /usr/bin/systemctl stop redwood-filter.service -q
#    /usr/bin/systemctl disable redwood-filter.service -q
#fi
%endif

# Post install
%post
%if "0%{dist}" == "0.v6"
/sbin/chkconfig --add redwood-filter >/dev/null 2>&1 || :
/sbin/service redwood-filter condrestart >/dev/null 2>&1 || :
#%else
#/sbin/chkconfig --add redwood-filter >/dev/null 2>&1 || :
#/usr/bin/systemctl enable redwood-filter.service -q
#/usr/bin/systemctl reload-or-restart redwood-filter.service -q
%endif

# Post uninstall
%postun
/sbin/ldconfig
%if "0%{dist}" == "0.v6"
if [ -f /var/lock/subsys/redwood-filter ]; then
    killall -TERM redwood-filter 2>&1 >/dev/null || :
    sleep 2
fi
%endif

# Files
%files
%defattr(-,root,root)
%if "0%{dist}" == "0.v6"
%attr(755,root,root) %{_sysconfdir}/init.d/redwood-filter
#%else
#%attr(755,root,root) /lib/systemd/system
#%attr(755,root,root) %{_tmpfilesdir}
%endif
%{_sbindir}/redwood-filter
%{_sysconfdir}/redwood
%attr(755,redwood,redwood) %{_localstatedir}/log/redwood

# vi: expandtab shiftwidth=4 softtabstop=4 tabstop=4
